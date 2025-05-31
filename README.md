# `go-future`

一个强大且泛型友好的 Go 异步处理库，灵感来自 JavaScript 的 Promise，支持链式调用、错误处理、并发组合、重试、超时等功能。

---

## ✨ 特性

* 基于泛型 `Future[T]` 实现，支持类型安全
* 类似 JS 的 `.Then()`, `.Catch()`, `.Finally()` 链式操作
* 提供 `All`, `Any`, `Race`, `AllSettled` 等组合工具
* 支持 `WithTimeout`, `Retry`, `Sleep` 等实用功能

---

## 🚀 安装

```bash
go get github.com/viocha/go-future
```

---

## 🔧 构造 Future 的方式

可以通过 `future` 包的顶层函数创建 `Future`。

| 构造方式      | 方法                                               | 描述                                          |
|-----------|--------------------------------------------------|---------------------------------------------|
| 1. 使用执行器  | `future.New`                                     | 传入 `resolve` 和 `reject` 回调执行器，适合标准异步处理      |
| 2. 带上下文取消 | `future.NewWithContext`                          | 可中断的 Future，监听 `context.Context` 超时或取消      |
| 3. 手动控制   | `future.NewResolvers`                            | 返回 `Future` 和对应的 `resolve/reject` 函数，适合外部触发 |
| 4. 包装同步函数 | `future.FromFunc`                                | 将普通函数包装为异步 Future，自动捕获 panic                |
| 5. 已解决/拒绝 | `future.Resolve`, `future.Reject`, `future.From` | 快速创建已确定状态的 Future                           |

示例：

```go
import (
	"fmt"
	"time"

	"github.com/viocha/go-future"
)

func main() {
	p := future.New(func(resolve func(int), reject func(error)) {
		time.Sleep(1 * time.Second)
		resolve(42)
	})

	val, err := p.Await()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("Value:", val)
}
```

---

## 📘 使用示例

### 基础链式调用

```go
import (
	"fmt"
	"log"
	"time"

	"github.com/viocha/go-future"
)

// ...
future.Resolve(1).
    MapStr(func(v int) string { return fmt.Sprintf("Result: %d", v) }). // 使用 MapT, MapInt, MapStr 等具体类型转换
    ThenStr(func(s string) *future.Future[int] { // 使用 ThenT, ThenInt, ThenStr 等具体类型转换
        fmt.Println(s)
        return future.Resolve(len(s))
    }).
    Catch(func(err error) {
        log.Println("Error:", err)
    }).
    Finally(func() {
        fmt.Println("All done")
    }).Await()
```

### 错误处理

```go
import (
	"errors"
	"fmt"

	"github.com/viocha/go-future"
)

// ...
p := future.New(func(_ func(any), reject func(error)) { // resolve 的类型取决于 Future 的泛型参数
    reject(errors.New("some error"))
})
p.Catch(func(err error) {
    fmt.Println("Caught error:", err)
}).Await()
```

### 等待所有 Future 完成

```go
import (
	"fmt"

	"github.com/viocha/go-future"
)

// ...
p1 := future.Resolve(1)
p2 := future.Resolve(2)

result, err := future.All(p1, p2).Await()
if err != nil {
    // handle error
}
fmt.Println(result) // [1 2]
```

---

## ⏱️ 实用工具

### 超时控制

```go
import (
	"fmt"
	"time"

	"github.com/viocha/go-future"
)

func longRunningTask() *future.Future[string] {
	return future.New(func(resolve func(string), reject func(error)) {
		time.Sleep(5 * time.Second) // 模拟耗时操作
		resolve("task completed")
	})
}

// ...
p := longRunningTask().WithTimeout(3 * time.Second)
val, err := p.Await()
if err != nil {
	fmt.Println("Error:", err) // Error: promise timed out: promise timed out after 3s
} else {
	fmt.Println("Value:", val)
}
```

### 重试机制

```go
import (
	"errors"
	"fmt"
	"time"

	"github.com/viocha/go-future"
)

var attemptCount = 0

func tryConnect() *future.Future[string] {
	return future.New(func(resolve func(string), reject func(error)) {
		attemptCount++
		fmt.Printf("Attempting connection #%d\n", attemptCount)
		if attemptCount < 3 {
			time.Sleep(50 * time.Millisecond)
			reject(errors.New("connection failed"))
		} else {
			resolve("connected successfully")
		}
	})
}

// ...
p := future.Retry(tryConnect, future.RetryOptions{Attempts: 5, Delay: 100 * time.Millisecond})
res, err := p.Await()
if err != nil {
    fmt.Println("Retry failed:", err)
} else {
    fmt.Println("Retry succeeded:", res)
}
```

### 睡眠

```go
import (
	"fmt"
	"time"

	"github.com/viocha/go-future"
)

// ...
future.Sleep(200 * time.Millisecond).ThenT(func(_ struct{}) *future.Future[string] {
    return future.Resolve("Done sleeping")
}).Try(func(s string) {
	fmt.Println(s)
}).Await()
```

---

## 🧩 组合函数

* `All(futures...)`: 全部成功返回结果，任何一个失败立即中止
* `Any(futures...)`: 任一成功立即返回，否则聚合错误
* `AllSettled(futures...)`: 全部执行完毕，返回每个的状态
* `Race(futures...)`: 返回最先完成的 Future（无论成功或失败）

---

## 🔄 状态控制与辅助函数

| 函数 (Future)                                                | 描述                      |
|------------------------------------------------------------|-------------------------|
| `Await()`                                                  | 阻塞直到完成，返回结果和错误          |
| `MustResolve()`                                            | 必须成功，否则 panic           |
| `MustReject()`                                             | 必须失败，否则 panic           |
| `Block()`                                                  | 阻塞直到 Future 完成，不返回结果和错误 |
| `Done()`                                                   | 获取结束的 chan              |
| `State()`                                                  | 获取当前 Future 的状态         |
| `IsPending()`, `IsFulfilled()`, `IsRejected()`, `IsDone()` | 查询状态                    |

---

## 📦 类型转换和链式映射

`Future[T]` 提供了多种 `Map` 和 `Then` 的变体，以便于类型转换：

* `MapT(f func(v T) T) *Future[T]`
* `MapInt(f func(v T) int) *Future[int]`
* `MapFloat(f func(v T) float64) *Future[float64]`
* `MapStr(f func(v T) string) *Future[string]`
* `MapBool(f func(v T) bool) *Future[bool]`

以及对应的 `Then` 系列：

* `ThenT(f func(T) *Future[T]) *Future[T]`
* `ThenInt(f func(T) *Future[int]) *Future[int]`
* `ThenFloat(f func(T) *Future[float64]) *Future[float64]`
* `ThenStr(f func(T) *Future[string]) *Future[string]`
* `ThenBool(f func(T) *Future[bool]) *Future[bool]`

示例：

```go
import (
	"fmt"

	"github.com/viocha/go-future"
)

// ...
p := future.Resolve(3)

p.MapStr(func(v int) string { // 从 int 映射到 string
    return fmt.Sprintf("num: %d", v)
}).ThenInt(func(s string) *future.Future[int] { // 从 string 映射到 Future[int]
    return future.Resolve(len(s))
}).Try(func(length int) {
	fmt.Printf("Final length: %d\n", length)
}).Await()
```

---

## 🛡️ 错误类型

* `ErrUnexpectedPanic`
* `ErrAllFailed`
* `ErrTimeout`
* `ErrRetryFailed`

## 📄 License

MIT License
