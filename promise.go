package promise

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/viocha/go-promise/common"
)

type promiseState int

const (
	Pending promiseState = iota
	Fulfilled
	Rejected
)

var (
	ErrNoResolveOrRejectCalled = fmt.Errorf("executor did not resolve or reject")
	ErrPanic                   = fmt.Errorf("panic occurred")
	ErrAllFailed               = fmt.Errorf("all promises failed")
	ErrTimeout                 = fmt.Errorf("promise timed out")
	ErrRetryFailed             = fmt.Errorf("retry failed, max attempts reached")
)

var wg sync.WaitGroup // 全局 WaitGroup，用于跟踪所有 Promise 的完成状态

// 阻塞直到所有创建的Promise完成
func Block() {
	wg.Wait()
}

// 返回一个通道，等待此时的所有 Promise 完成时关闭
func Done() chan struct{} {
	return common.GetWgChan(&wg)
}

type Promise[T any] struct {
	lock  sync.RWMutex
	state promiseState
	value T
	err   error
	done  chan struct{}
}

type Executor[T any] func(resolve func(T), reject func(error))

func getFns[T any](p *Promise[T], executor Executor[T]) (func(T), func(error), func()) {
	resolve := func(v T) {
		p.lock.Lock()
		defer p.lock.Unlock()
		if p.state != Pending {
			return
		}
		p.state = Fulfilled
		p.value = v
		close(p.done)
	}

	reject := func(err error) {
		p.lock.Lock()
		defer p.lock.Unlock()
		if p.state != Pending {
			return
		}
		p.state = Rejected
		p.err = err
		close(p.done)
	}

	exec := func() {
		defer func() {
			if r := recover(); r != nil && p.state == Pending {
				reject(common.WrapMsg(ErrPanic, "panic in executor: %v", toError(r)))
			}
			if p.state == Pending {
				reject(ErrNoResolveOrRejectCalled)
			}
		}()
		executor(resolve, reject)
	}

	return resolve, reject, exec
}

func New[T any](executor Executor[T]) *Promise[T] {
	p := &Promise[T]{
		state: Pending,
		done:  make(chan struct{}),
	}

	wg.Add(1) // 每创建一个 Promise，就增加 WaitGroup 的计数
	go func() {
		<-p.done
		wg.Done() // 当 Promise 完成时，减少 WaitGroup 的计数
	}()

	_, _, exec := getFns(p, executor)
	go exec()
	return p
}

func NewWithContext[T any](ctx context.Context, executor func(resolve func(T), reject func(error))) *Promise[T] {
	p := &Promise[T]{
		state: Pending,
		done:  make(chan struct{}),
	}

	wg.Add(1)
	go func() {
		select {
		case <-p.done:
			// 正常完成
		case <-ctx.Done():
			// 被取消
			p.lock.Lock()
			if p.state == Pending {
				p.state = Rejected
				p.err = ctx.Err() // context.Canceled 或 context.DeadlineExceeded
				close(p.done)
			}
			p.lock.Unlock()
		}
		wg.Done()
	}()

	_, _, exec := getFns(p, executor)
	go exec()
	return p
}

func WithResolvers[T any]() (*Promise[T], func(T), func(error)) {
	p := &Promise[T]{
		state: Pending,
		done:  make(chan struct{}),
	}

	wg.Add(1) // 每创建一个 Promise，就增加 WaitGroup 的计数
	go func() {
		<-p.done
		wg.Done() // 当 Promise 完成时，减少 WaitGroup 的计数
	}()

	resolve, reject, _ := getFns(p, nil)
	return p, resolve, reject
}

func FromFunc[T any](f func() T) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		var result T
		didPanic := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					reject(toError(r)) // 只能通过panic传递错误
					didPanic = true
				}
			}()
			result = f()
		}()

		if !didPanic {
			resolve(result)
		}
	})
}

// 如果传入的值是 error 类型，则直接返回该 error，否则将其包装为 error 类型
func toError(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return fmt.Errorf("%v", r)
}

func (p *Promise[T]) Try(f func(v T)) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()

		if err != nil {
			reject(err)
			return
		}

		didPanic := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					reject(common.WrapMsg(ErrPanic, "panic in Try: %v", toError(r)))
					didPanic = true
				}
			}()
			f(val)
		}()

		if !didPanic {
			resolve(val)
		}
	})
}

func (p *Promise[T]) Catch(f func(err error)) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()

		if err == nil {
			resolve(val)
			return
		}

		didPanic := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					reject(common.WrapMsg(ErrPanic, "panic in Catch: %v", toError(r)))
					didPanic = true
				}
			}()
			f(err)
		}()
		if !didPanic {
			reject(err)
		}
	})
}

func (p *Promise[T]) Finally(f func()) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()

		didPanic := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					reject(common.WrapMsg(ErrPanic, "panic in Finally: %v", toError(r)))
					didPanic = true
				}
			}()
			f()
		}()

		if didPanic {
			return
		}
		if err == nil {
			resolve(val)
		} else {
			reject(err)
		}
	})
}

// 阻塞直到 Promise 完成，并返回结果和错误
func (p *Promise[T]) Await() (T, error) {
	<-p.done
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.value, p.err
}

func (p *Promise[T]) MustResolve() T {
	val, err := p.Await()
	if err != nil {
		panic(err)
	}
	return val
}

func (p *Promise[T]) MustReject() error {
	_, err := p.Await()
	if err == nil {
		panic("expected promise to be rejected, but it was resolved")
	}
	return err
}

func (p *Promise[T]) MapT(f func(v T) T) *Promise[T]                 { return Map(p, f) }
func (p *Promise[T]) MapInt(f func(v T) int) *Promise[int]           { return Map(p, f) }
func (p *Promise[T]) MapFloat(f func(v T) float64) *Promise[float64] { return Map(p, f) }
func (p *Promise[T]) MapString(f func(v T) string) *Promise[string]  { return Map(p, f) }
func (p *Promise[T]) MapBool(f func(v T) bool) *Promise[bool]        { return Map(p, f) }

// 执行新的Promise
func (p *Promise[T]) ThenT(next func(T) *Promise[T]) *Promise[T]       { return Then(p, next) }
func (p *Promise[T]) ThenInt(next func(T) *Promise[int]) *Promise[int] { return Then(p, next) }
func (p *Promise[T]) ThenFloat(next func(T) *Promise[float64]) *Promise[float64] {
	return Then(p, next)
}
func (p *Promise[T]) ThenString(next func(T) *Promise[string]) *Promise[string] { return Then(p, next) }
func (p *Promise[T]) ThenBool(next func(T) *Promise[bool]) *Promise[bool]       { return Then(p, next) }

// 失败时执行新的Promise
func (p *Promise[T]) Else(next func(error) *Promise[T]) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()

		if err == nil {
			resolve(val)
			return
		}

		newPromise := next(err)
		result, err := newPromise.Await()
		if err == nil {
			resolve(result)
		} else {
			reject(err)
		}
	})
}

// 设置超时，如果Promise在指定时间内未完成，则返回错误
func (p *Promise[T]) WithTimeout(d time.Duration) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		timeout := time.After(d)
		select {
		case <-p.done:
			val, err := p.Await()
			if err == nil {
				resolve(val)
			} else {
				reject(err)
			}
		case <-timeout:
			reject(common.WrapMsg(ErrTimeout, "promise timed out after %s", d))
		}
	})
}

// ======================================== 静态方法 ========================================

// 等待所有Promise成功，任一失败立即返回错误
func All[T any](promises ...*Promise[T]) *Promise[[]T] {
	return New(func(resolve func([]T), reject func(error)) {
		results := make([]T, len(promises))
		var wg sync.WaitGroup
		wg.Add(len(promises)) // 等待所有Promise完成
		done := common.GetWgChan(&wg)

		var once sync.Once           // 确保只接收一次错误
		errCh := make(chan error, 1) // 接收第一个错误

		for i, p := range promises { // 使用索引访问，确保并发安全
			go func() {
				defer wg.Done() // 每个Promise完成时减少计数
				val, err := p.Await()
				if err == nil {
					results[i] = val // 成功时保存结果
					return
				}
				once.Do(func() {
					errCh <- err // 发送错误
					close(errCh)
				})
			}()
		}

		select {
		case err := <-errCh:
			reject(err)
		case <-done:
			resolve(results)
		}
	})
}

// 任意一个成功即返回，全部失败返回聚合错误
func Any[T any](promises ...*Promise[T]) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		var wg sync.WaitGroup
		wg.Add(len(promises))
		done := common.GetWgChan(&wg)

		errs := make([]error, len(promises)) // 用于收集所有错误
		resultCh := make(chan T, 1)          // 用于发送第一个成功的结果
		once := sync.Once{}

		for i, p := range promises {
			go func() {
				defer wg.Done()
				val, err := p.Await()
				if err != nil {
					errs[i] = err // 保存错误
					return
				}
				once.Do(func() {
					resultCh <- val // 发送第一个成功的结果
					close(resultCh)
				})
			}()
		}

		select {
		case result := <-resultCh:
			resolve(result) // 成功时返回结果
		case <-done:
			reject(common.WrapSub(ErrAllFailed, errors.Join(errs...), "all promises failed in Any"))
		}
	})
}

type Result[T any] struct {
	Val T
	Err error
}

func AllSettled[T any](promises ...*Promise[T]) *Promise[[]Result[T]] {
	return New(func(resolve func([]Result[T]), reject func(error)) {
		results := make([]Result[T], len(promises))
		var wg sync.WaitGroup
		wg.Add(len(promises)) // 等待所有Promise完成
		done := common.GetWgChan(&wg)

		for i, p := range promises {
			go func() {
				defer wg.Done() // 每个Promise完成时减少计数
				val, err := p.Await()
				results[i] = Result[T]{Val: val, Err: err}
			}()
		}

		<-done           // 等待所有Promise完成
		resolve(results) // 返回所有结果
	})
}

// 获取第一个完成的Promise结果（可能成功/失败）
func Race[T any](promises ...*Promise[T]) *Promise[T] {
	return New(func(resolve func(T), reject func(error)) {
		once := sync.Once{}
		done := make(chan struct{})

		for _, p := range promises {
			go func() {
				val, err := p.Await()
				once.Do(func() {
					if err != nil {
						reject(err)
					} else {
						resolve(val)
					}
					close(done)
				})
			}()
		}

		<-done
	})
}

// 快速创建已解决的Promise
func Resolve[T any](value T) *Promise[T] {
	return New(func(resolve func(T), _ func(error)) {
		resolve(value)
	})
}

// 快速创建已拒绝的Promise
func Reject[T any](err error) *Promise[T] {
	return New(func(_ func(T), reject func(error)) {
		reject(err)
	})
}

type RetryOptions struct {
	MaxAttempts int                             // 最大重试次数，默认3次，-1表示无限重试
	Delay       time.Duration                   // 固定的重试间隔时间，默认100毫秒
	DelayFn     func(attempt int) time.Duration // 可选的自定义延迟函数，attempt从1开始
}

// 重试执行任务，直到成功或达到最大重试次数
func Retry[T any](task func() *Promise[T], retryOptions ...RetryOptions) *Promise[T] {
	options := common.ParseOptional(retryOptions, RetryOptions{})
	if options.Delay == 0 {
		options.Delay = 100 * time.Millisecond // 默认延迟100毫秒
	}
	if options.MaxAttempts == 0 {
		options.MaxAttempts = 3 // 默认最大重试次数为3
	}
	return New(func(resolve func(T), reject func(error)) {
		attempt := 0
		for {
			p := task()
			val, err := p.Await()
			if err == nil {
				resolve(val)
				return
			}

			attempt++
			if options.MaxAttempts >= 0 && attempt >= options.MaxAttempts {
				reject(common.WrapSub(ErrRetryFailed, err, "max retry attempts (%d) reached", options.MaxAttempts))
				return
			}

			nextDelay := options.Delay
			if options.DelayFn != nil {
				nextDelay = options.DelayFn(attempt)
			}
			time.Sleep(nextDelay)
		}
	})
}

// =========================================== 转换函数 ========================================

func Map[T, R any](p *Promise[T], f func(v T) R) *Promise[R] {
	return New(func(resolve func(R), reject func(error)) {
		val, err := p.Await()

		if err != nil {
			reject(err)
			return
		}

		var result R
		didPanic := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					reject(common.WrapMsg(ErrPanic, "panic in Map: %v", toError(r)))
					didPanic = true
				}
			}()
			result = f(val)
		}()
		if !didPanic {
			resolve(result)
		}
	})
}

func Then[T, R any](p *Promise[T], next func(T) *Promise[R]) *Promise[R] {
	return New(func(resolve func(R), reject func(error)) {
		val, err := p.Await()
		if err != nil {
			reject(err)
			return
		}

		newPromise := next(val)
		result, err := newPromise.Await()
		if err == nil {
			resolve(result)
		} else {
			reject(err)
		}
	})
}

// =========================================== 工具函数 ========================================

// 创建延迟Promise
func Sleep(d time.Duration) *Promise[struct{}] {
	return New(func(resolve func(struct{}), _ func(error)) {
		time.Sleep(d)
		resolve(struct{}{})
	})
}

// 强制获取值，如果有错误则抛出 panic
func MustGet[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// 强制获取两个值，如果有错误则抛出 panic
func MustGet2[T1, T2 any](v1 T1, v2 T2, err error) (T1, T2) {
	if err != nil {
		panic(err)
	}
	return v1, v2
}
