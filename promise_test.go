package promise

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func TestPromise(t *testing.T) {
	t.Parallel() // 并行执行测试
	New(func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		if rand.Intn(2) == 0 {
			resolve(2) // 成功
		} else {
			reject(errors.New("something went wrong")) // 失败
		}
	}).MapT(func(v int) int {
		return v * 2 // 将值乘以2
	}).Try(func(v int) {
		fmt.Println("1: Resolved with value:", v) // 如果成功，输出4
	}).Try(func(v int) {
		fmt.Println("2: Resolved with value:", v) // 如果成功，输出4
	}).Catch(func(err error) {
		fmt.Println("Rejected with error:", err) // 如果失败，输出错误信息
	}).Finally(func() {
		fmt.Println("Promise completed") // 无论成功或失败都会执行
	})

	<-Done()
}

func TestNewWithContext(t *testing.T) {
	t.Parallel() // 并行执行测试
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel() // 确保在测试结束时取消上下文

	go func() {
		time.Sleep(50 * time.Millisecond) // 模拟异步操作
		cancel()                          // 取消上下文
	}()

	NewWithContext(ctx, func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		resolve(2) // 成功
	}).MapT(func(v int) int {
		return v * 2 // 将值乘以2
	}).Try(func(v int) {
		fmt.Println("Resolved with value:", v) // 如果成功，输出4
	}).Catch(func(err error) {
		fmt.Println("Rejected with error:", err) // 如果失败，输出错误信息
	})

	<-Done() // 等待所有异步操作完成
}

func TestResolve(t *testing.T) {
	t.Parallel() // 并行执行测试
	Resolve(42).Try(func(v int) {
		fmt.Println("Resolved with value:", v) // 如果成功，输出42
	}).Catch(func(err error) {
		fmt.Println("Rejected with error:", err) // 如果失败，输出错误信息
	})
	<-Done() // 等待所有异步操作完成
}

func TestReject(t *testing.T) {
	t.Parallel() // 并行执行测试
	Reject[int](errors.New("something went wrong")).Try(func(v int) {
		fmt.Println("Resolved with value:", v) // 如果成功，不会执行
	}).Catch(func(err error) {
		fmt.Println("Rejected with error:", err) // 如果失败，输出错误信息
	})
	<-Done() // 等待所有异步操作完成
}

func TestPromisePanic(t *testing.T) {
	t.Parallel() // 并行执行测试
	New(func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		if rand.Intn(2) == 0 {
			resolve(2) // 成功
		} else {
			panic(errors.New("panic reason")) // 失败
		}
	}).MapT(func(v int) int {
		return v * 2 // 将值乘以2
	}).Try(func(v int) {
		fmt.Println("Resolved with value:", v) // 如果成功，输出4
	}).Catch(func(err error) {
		fmt.Println("Rejected with error:", err) // 如果失败，输出错误信息
	}).Finally(func() {
		fmt.Println("Promise completed") // 无论成功或失败都会执行
	})

	Block()
}

func TestPromiseAll(t *testing.T) {
	t.Parallel() // 并行执行测试
	p1 := New(func(resolve func(int), _ func(error)) {
		time.Sleep(50 * time.Millisecond)
		resolve(1)
	})
	p2 := New(func(resolve func(int), _ func(error)) {
		time.Sleep(100 * time.Millisecond)
		resolve(2)
	})
	p3 := New(func(resolve func(int), _ func(error)) {
		time.Sleep(150 * time.Millisecond)
		resolve(3)
	})
	All(p1, p2, p3).Try(func(vals []int) {
		fmt.Println("All resolved:", vals)
		if len(vals) != 3 || vals[0] != 1 || vals[1] != 2 || vals[2] != 3 {
			t.Error("All result mismatch")
		}
	}).Catch(func(err error) {
		t.Error("All should not fail:", err)
	}).Finally(func() {
		fmt.Println("All completed")
	})

	Block()
}

func TestPromiseAny(t *testing.T) {
	t.Parallel() // 并行执行测试
	p1 := New(func(resolve func(int), reject func(error)) {
		time.Sleep(300 * time.Millisecond)
		// resolve(1)
		reject(errors.New("p1 failed"))
	})
	p2 := New(func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		// resolve(2)
		reject(errors.New("p2 failed"))
	})
	p3 := New(func(resolve func(int), reject func(error)) {
		time.Sleep(10 * time.Millisecond)
		// resolve(3)
		reject(errors.New("p3 failed"))
	})
	Any(p1, p2, p3).Try(func(val int) {
		fmt.Println("All resolved:", val)
		if val != 3 {
			t.Error("Any result mismatch")
		}
	}).Catch(func(err error) {
		t.Log("Any failed:", err)
	}).Finally(func() {
		t.Log("Any completed")
	})

	Block()
}

func TestPromiseRace(t *testing.T) {
	t.Parallel() // 并行执行测试
	p1 := New(func(resolve func(int), _ func(error)) {
		time.Sleep(300 * time.Millisecond)
		resolve(1)
	})
	p2 := New(func(resolve func(int), _ func(error)) {
		time.Sleep(200 * time.Millisecond)
		resolve(2)
	})
	p3 := New(func(_ func(int), reject func(error)) {
		time.Sleep(10 * time.Millisecond)
		reject(errors.New("p3 failed"))
	})
	Race(p1, p2, p3).Try(func(val int) {
		t.Logf("Resolved with value: %d", val)
	}).Catch(func(err error) {
		t.Logf("Rejected with error: %v", err)
	}).Finally(func() {
		t.Log("Race completed")
	})

	Block()
}

func TestRetry(t *testing.T) {
	t.Parallel() // 并行执行测试
	Retry(func() *Promise[int] {
		return New(func(resolve func(int), reject func(error)) {
			if rand.Intn(3) == 0 {
				resolve(666) // 成功
			} else {
				reject(errors.New("random failure")) // 失败
			}
		})
	}).Try(func(val int) {
		t.Logf("Retry resolved with value: %d", val) // 如果成功，输出值
	}).Catch(func(err error) {
		t.Logf("Retry failed with error: %v", err) // 如果失败，输出错误信息
	}).Finally(func() {
		t.Log("Retry completed") // 无论成功或失败都会执行
	})

	Block()
}

func TestSleep(t *testing.T) {
	t.Parallel() // 并行执行测试
	Sleep(500 * time.Millisecond).MustResolve()
	t.Log("Slept for 500 milliseconds") // 确认睡眠完成
}

func TestThen(t *testing.T) {
	t.Parallel() // 并行执行测试
	New(func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		if rand.Intn(3) == 0 {
			resolve(10) // 成功
		} else {
			reject(errors.New("reject reason")) // 失败
		}
	}).ThenT(func(v int) *Promise[int] {
		return New(func(resolve func(int), _ func(error)) {
			resolve(v * 2) // 将值乘以2
		})
	}).Try(func(v int) {
		t.Logf("Then resolved with value: %d", v) // 如果成功，输出值
	}).Catch(func(err error) {
		t.Logf("Then failed with error: %v", err) // 如果失败，输出错误信息
	}).Finally(func() {
		t.Log("Then completed") // 无论成功或失败都会执行
	})

	Block()
}

func TestPromise_Else(t *testing.T) {
	t.Parallel() // 并行执行测试
	New(func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		if rand.Intn(3) == 0 {
			resolve(42) // 成功
		} else {
			reject(errors.New("something went wrong")) // 失败
		}
	}).Else(func(err error) *Promise[int] {
		return New(func(resolve func(int), _ func(error)) {
			t.Logf("Handling error: %v", err) // 处理错误
			resolve(100)                      // 返回一个默认值
		})
	}).Try(func(v int) {
		t.Logf("Resolved with value: %d", v) // 如果成功，输出42；如果失败，输出100
	}).Catch(func(err error) {
		t.Logf("Rejected with error: %v", err) // 如果失败，输出错 误信息
	}).Finally(func() {
		t.Log("Promise completed") // 无论成功或失败都会执行
	})

	Block()
}

func TestWithResolvers(t *testing.T) {
	t.Parallel() // 并行执行测试
	p, resolve, reject := WithResolvers[int]()

	go func() {
		time.Sleep(200 * time.Millisecond)
		if rand.Intn(2) == 0 {
			resolve(42) // 成功
		} else {
			reject(errors.New("something went wrong")) // 失败
		}
	}()

	p.Try(func(v int) {
		t.Logf("Resolved with value: %d", v) // 如果成功，输出42
	}).Catch(func(err error) {
		t.Logf("Rejected with error: %v", err) // 如果失败，输出错误信息
	}).Finally(func() {
		t.Log("Promise completed") // 无论成功或失败都会执行
	}).Await()
}

func TestWithFunc(t *testing.T) {
	t.Parallel()
	FromFunc(func() int {
		time.Sleep(200 * time.Millisecond)
		if rand.Intn(2) == 0 {
			return 42 // 成功
		}
		panic(errors.New("something went wrong")) // 失败
	}).Try(func(v int) {
		t.Logf("Resolved with value: %d", v) // 如果成功，输出42
	}).Catch(func(err error) {
		t.Logf("Rejected with error: %v", err) // 如果失败，输出错误信息
	}).Finally(func() {
		t.Log("Promise completed") // 无论成功或失败都会执行
	}).Await()
}

func TestAllSettled(t *testing.T) {
	t.Parallel() // 并行执行测试
	p1 := New(func(resolve func(int), reject func(error)) {
		time.Sleep(300 * time.Millisecond)
		// resolve(1)
		reject(errors.New("p1 failed"))
	})
	p2 := New(func(resolve func(int), reject func(error)) {
		time.Sleep(200 * time.Millisecond)
		resolve(2)
		// reject(errors.New("p2 failed"))
	})
	p3 := New(func(resolve func(int), reject func(error)) {
		time.Sleep(10 * time.Millisecond)
		// resolve(3)
		reject(errors.New("p3 failed"))
	})
	AllSettled(p1, p2, p3).Try(func(results []Result[int]) {
		fmt.Println("AllSettled resolved:", results)
	}).Catch(func(err error) {
		t.Log("AllSettled failed:", err)
	}).Finally(func() {
		t.Log("AllSettled completed")
	})

	Block()
}

func TestMustGet(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when calling MustGet on an error Result, but did not panic")
		}
	}()

	MustGet(1, errors.New("test error"))
}

func TestMustGet2(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when calling MustGet2 on an error Result, but did not panic")
		}
	}()

	MustGet2(1, 2, errors.New("test error"))
}
