package future

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/viocha/go-future/internal/common"
)

// 用于只关心是否成功，不关心返回结果的异步任务
type Task = Future[struct{}]

type State int

const (
	Pending State = iota
	Fulfilled
	Rejected
)

var (
	ErrMust        = fmt.Errorf("expected panic occurred") // 用于标识预期的 panic，会被捕获
	ErrAllFailed   = fmt.Errorf("all promises failed")
	ErrTimeout     = fmt.Errorf("promise timed out")
	ErrRetryFailed = fmt.Errorf("retry failed, max attempts reached")
)

type Future[T any] struct {
	lock  sync.Mutex
	state State
	value T
	err   error
	done  chan struct{}
}

type Executor[T any] func(resolve func(T), reject func(error))
type ExecutorWithContext[T any] func(ctx context.Context, resolve func(T), reject func(error))

func getResolvers[T any](p *Future[T]) (func(T), func(error)) {
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
	return resolve, reject
}

func getExecutorDeferFn(reject func(error)) func() {
	return func() {
		if r := recover(); r != nil {
			if r, ok := r.(error); ok {
				if errors.Is(r, ErrMust) { // 预期的 panic
					reject(r)
				}
			}
			panic(r) // 非预期的 panic，继续抛出
		}
	}
}

func getFns[T any](p *Future[T], executor Executor[T]) (func(T), func(error), func()) {
	// 只有在 resolve/reject中会修改state，而其他地方的读取操作必然会调用Await，故其他地方不需要加锁，
	resolve, reject := getResolvers(p)

	exec := func() {
		defer getExecutorDeferFn(reject)()
		executor(resolve, reject)
	}

	return resolve, reject, exec
}

func getFnsWithContext[T any](ctx context.Context, p *Future[T], executor ExecutorWithContext[T]) (func(T), func(error),
	func()) {
	resolve, reject := getResolvers(p)

	exec := func() {
		defer getExecutorDeferFn(reject)()
		go func() { // 防止executor不监听ctx.Done()并正确reject
			select {
			case <-ctx.Done():
				reject(ctx.Err()) // 如果上下文结束，返回错误
			case <-p.done:
				return // 如果 Future 已经完成，则不再执行
			}
		}()
		executor(ctx, resolve, reject)
	}
	return resolve, reject, exec
}

func New[T any](executor Executor[T]) *Future[T] {
	p := &Future[T]{
		state: Pending,
		done:  make(chan struct{}),
	}
	_, _, exec := getFns(p, executor)
	go exec()
	return p
}

func NewWithContext[T any](ctx context.Context, executor ExecutorWithContext[T]) *Future[T] {
	p := &Future[T]{
		state: Pending,
		done:  make(chan struct{}),
	}
	_, _, exec := getFnsWithContext(ctx, p, executor)
	go exec()
	return p
}

func FromFunc[T any](f func() T) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		if err := DoWithPanic(func() {
			resolve(f())
		}); err != nil {
			reject(err)
		}
	})
}

func FromFuncWithContext[T any](ctx context.Context, f func(ctx context.Context) T) *Future[T] {
	return NewWithContext(ctx, func(ctx context.Context, resolve func(T), reject func(error)) {
		if err := DoWithPanic(func() {
			resolve(f(ctx))
		}); err != nil {
			reject(err)
		}
	})
}

func NewResolvers[T any]() (*Future[T], func(T), func(error)) {
	p := &Future[T]{
		state: Pending,
		done:  make(chan struct{}),
	}

	resolve, reject, _ := getFns(p, nil)
	return p, resolve, reject
}

// 快速创建已解决的Future
func Resolve[T any](value T) *Future[T] {
	return New(func(resolve func(T), _ func(error)) {
		resolve(value)
	})
}

// 快速创建已拒绝的Future
func Reject[T any](err error) *Future[T] {
	return New(func(_ func(T), reject func(error)) {
		reject(err)
	})
}

// 当err为nil时，创建一个已解决的Future，否则创建一个已拒绝的Future
func From[T any](val T, err error) *Future[T] {
	if err != nil {
		return Reject[T](err)
	}
	return Resolve(val)
}

// ======================================== 查询 ========================================

// 获取当前 Future 的状态
func (p *Future[T]) State() State      { return p.state }
func (p *Future[T]) IsPending() bool   { return p.state == Pending }
func (p *Future[T]) IsFulfilled() bool { return p.state == Fulfilled }
func (p *Future[T]) IsRejected() bool  { return p.state == Rejected }
func (p *Future[T]) IsDone() bool      { return p.state != Pending }

// 获取结束的chan
func (p *Future[T]) Done() <-chan struct{} { return p.done }

// 阻塞直到 Future 完成，并返回结果和错误
func (p *Future[T]) Await() (T, error) {
	<-p.done
	return p.value, p.err
}

// 阻塞直到 Future 完成，不返回结果和错误
func (p *Future[T]) Block() {
	<-p.done
}

// 等待并获取结果，如果发生错误，则执行可捕获的panic
func (p *Future[T]) MustResolve() T {
	val, err := p.Await()
	if err != nil {
		panic(WrapMust(err))
	}
	return val
}

// 等待并获取错误，如果未发生错误，则执行可捕获的panic
func (p *Future[T]) MustReject() error {
	_, err := p.Await()
	if err == nil {
		panic(WrapMust("expected promise to be rejected, but it was resolved"))
	}
	return err
}

// ======================================== 试探并执行 ========================================

// 尝试获取值，如果结束且成功则返回值，否则返回默认值
func (p *Future[T]) GetOr(defaultValue T) T {
	if p.IsFulfilled() {
		return p.MustResolve()
	}
	return defaultValue
}

func (p *Future[T]) GetOrFunc(f func() T) T {
	if p.IsFulfilled() {
		return p.MustResolve()
	}
	return f()
}

// ======================================== 链式方法 ========================================

// 当 Future 成功时，异步执行f，并返回一个新的 Future
func (p *Future[T]) Try(f func(v T)) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()
		if err != nil {
			reject(err)
			return
		}

		if fErr := DoWithPanic(func() {
			f(val)
		}); fErr != nil {
			reject(fErr)
		} else {
			resolve(val)
		}
	})
}

// 当 Future 失败时，异步执行 f，并返回一个新的 Future
func (p *Future[T]) Catch(f func(err error)) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()
		if err == nil {
			resolve(val) // 如果没有错误，直接返回原值
			return
		}

		if fErr := DoWithPanic(func() {
			f(err)
		}); fErr != nil {
			reject(fErr) // 如果处理错误时发生错误，返回新的错误
		} else {
			reject(err) // 否则，返回原始错误
		}
	})
}

// 当 Future 完成时，无论成功或失败，异步执行 f，并返回一个新的 Future
func (p *Future[T]) Finally(f func()) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		_, _ = p.Await() // 等待 Future 完成
		if fErr := DoWithPanic(f); fErr != nil {
			reject(fErr)
		} else {
			p.Forward(resolve, reject)
		}
	})
}

// 同步等待当前Future被解决，然后将p的状态转发到新的 Future
func (p *Future[T]) Forward(resolve func(T), reject func(error)) {
	val, err := p.Await()
	if err == nil {
		resolve(val)
	} else {
		reject(err)
	}
}

// 成功时，将值进行转换
func (p *Future[T]) MapT(f func(v T) T) *Future[T]                 { return Map(p, f) }
func (p *Future[T]) MapInt(f func(v T) int) *Future[int]           { return Map(p, f) }
func (p *Future[T]) MapFloat(f func(v T) float64) *Future[float64] { return Map(p, f) }
func (p *Future[T]) MapStr(f func(v T) string) *Future[string]     { return Map(p, f) }
func (p *Future[T]) MapBool(f func(v T) bool) *Future[bool]        { return Map(p, f) }

// 成功时，执行新的Future
func (p *Future[T]) ThenT(f func(T) *Future[T]) *Future[T]                 { return Then(p, f) }
func (p *Future[T]) ThenInt(f func(T) *Future[int]) *Future[int]           { return Then(p, f) }
func (p *Future[T]) ThenFloat(f func(T) *Future[float64]) *Future[float64] { return Then(p, f) }
func (p *Future[T]) ThenStr(f func(T) *Future[string]) *Future[string]     { return Then(p, f) }
func (p *Future[T]) ThenBool(f func(T) *Future[bool]) *Future[bool]        { return Then(p, f) }

// 失败时，执行新的Future
func (p *Future[T]) Else(f func(error) *Future[T]) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()
		if err == nil {
			resolve(val) // 如果没有错误，直接返回原值
			return
		}

		f(err).Forward(resolve, reject) // 转发新的 Future 的结果
	})
}

// 失败时，将错误转换为值
func (p *Future[T]) ElseMap(f func(error) T) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		val, err := p.Await()
		if err == nil {
			resolve(val) // 如果没有错误，直接返回原值
			return
		}

		if fErr := DoWithPanic(func() {
			resolve(f(err)) // 使用 f 转换错误为值
		}); fErr != nil {
			reject(fErr) // 如果转换过程中发生错误，返回新的错误
		}
	})
}

// ========================================= 添加超时和context ========================================

// 设置超时，如果Future在指定时间内未完成，则返回错误。注意：不会影响executor的执行。
func (p *Future[T]) WithTimeout(d time.Duration) *Future[T] {
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

// 使用上下文来控制Future的取消。注意：不会影响executor的执行。
// 如果要支持取消executor，应该使用 NewWithContext
func (p *Future[T]) WithContext(ctx context.Context) *Future[T] {
	return New(func(resolve func(T), reject func(error)) {
		select {
		case <-ctx.Done():
			reject(ctx.Err()) // 如果上下文结束，返回错误
		case <-p.done:
			val, err := p.Await()
			if err == nil {
				resolve(val)
			} else {
				reject(err)
			}
		}
	})
}

// ======================================== 并发执行的函数 ========================================

// 等待所有Future成功，任一失败立即返回错误
func All[T any](promises ...*Future[T]) *Future[[]T] {
	return New(func(resolve func([]T), reject func(error)) {
		results := make([]T, len(promises))
		var wg sync.WaitGroup
		wg.Add(len(promises)) // 等待所有Future完成
		done := common.GetWgChan(&wg)

		var once sync.Once           // 确保只接收一次错误
		errCh := make(chan error, 1) // 接收第一个错误

		for i, p := range promises { // 使用索引访问，确保并发安全
			go func() {
				defer wg.Done() // 每个Future完成时减少计数
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
func Any[T any](promises ...*Future[T]) *Future[T] {
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
			reject(common.WrapSub(errors.Join(errs...), ErrAllFailed, "all promises failed in Any"))
		}
	})
}

type Result[T any] struct {
	Val T
	Err error
}

func AllSettled[T any](promises ...*Future[T]) *Future[[]Result[T]] {
	return New(func(resolve func([]Result[T]), reject func(error)) {
		results := make([]Result[T], len(promises))
		var wg sync.WaitGroup
		wg.Add(len(promises)) // 等待所有Future完成
		done := common.GetWgChan(&wg)

		for i, p := range promises {
			go func() {
				defer wg.Done() // 每个Future完成时减少计数
				val, err := p.Await()
				results[i] = Result[T]{Val: val, Err: err}
			}()
		}

		<-done           // 等待所有Future完成
		resolve(results) // 返回所有结果
	})
}

// 获取第一个完成的Future结果（可能成功/失败）
func Race[T any](promises ...*Future[T]) *Future[T] {
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

type RetryOptions struct {
	Attempts int                             // 最大重试次数
	Delay    time.Duration                   // 固定的重试间隔时间
	DelayFn  func(attempt int) time.Duration // 可选的自定义延迟函数，attempt从1开始
}

// =================================== 重试执行任务 ========================================

// 重试执行任务，直到成功或达到最大重试次数。retryOptions为可选参数，如果不提供，默认最大重试次数为3次，延迟1000毫秒
func Retry[T any](task func() *Future[T], retryOptions ...RetryOptions) *Future[T] {
	options := common.ParseOptional(retryOptions, RetryOptions{})
	if options.Delay == 0 {
		options.Delay = 1000 * time.Millisecond
	}
	if options.Attempts == 0 {
		options.Attempts = 3
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
			if options.Attempts >= 0 && attempt >= options.Attempts {
				reject(common.WrapSub(err, ErrRetryFailed, "max retry attempts (%d) reached", options.Attempts))
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

func Map[T, R any](p *Future[T], f func(v T) R) *Future[R] {
	return New(func(resolve func(R), reject func(error)) {
		val, err := p.Await()

		if err != nil {
			reject(err)
			return
		}

		if fErr := DoWithPanic(func() {
			resolve(f(val))
		}); fErr != nil {
			reject(fErr)
		}
	})
}

func Then[T, R any](p *Future[T], next func(T) *Future[R]) *Future[R] {
	return New(func(resolve func(R), reject func(error)) {
		val, err := p.Await()
		if err != nil {
			reject(err)
			return
		}

		next(val).Forward(resolve, reject)
	})
}

// =========================================== 工具函数 ========================================

// 创建延迟Future
func Sleep(d time.Duration) *Future[struct{}] {
	return New(func(resolve func(struct{}), _ func(error)) {
		time.Sleep(d)
		resolve(struct{}{})
	})
}

// 将一个值包装成ErrMustFnPanic错误，并返回错误
func WrapMust(v any) error {
	return errors.Join(ErrMust, common.ToError(v))
}

// 强制获取值，如果有错误则 panic
func MustGet[T any](v T, err error) T {
	if err != nil {
		panic(common.WrapSub(err, ErrMust, "panic in MustGet"))
	}
	return v
}

// 强制获取两个值，如果有错误则 panic
func MustGet2[T1, T2 any](v1 T1, v2 T2, err error) (T1, T2) {
	if err != nil {
		panic(common.WrapSub(err, ErrMust, "panic in MustGet2"))
	}
	return v1, v2
}

// 捕获 ErrMust 错误的panic
func DoWithPanic(f func()) error {
	return common.DoSafe(f, ErrMust)
}
