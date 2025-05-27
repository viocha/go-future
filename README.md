# `go-promise`

一个强大且泛型友好的 Go 异步处理库，灵感来自 JavaScript 的 Promise，支持链式调用、错误处理、并发组合、重试、超时等功能。

---

## ✨ 特性

* 基于泛型 `Promise[T]` 实现，支持类型安全
* 类似 JS 的 `.Then()`, `.Catch()`, `.Finally()` 链式操作
* 提供 `All`, `Any`, `Race`, `AllSettled` 等组合工具
* 支持 `WithTimeout`, `Retry`, `Sleep` 等实用功能

---

## 🚀 安装

```bash
go get github.com/viocha/go-promise
```

---

## 🔧 构造 Promise 的方式（4 种）

| 构造方式      | 方法                       | 描述                                           |
|-----------|--------------------------|----------------------------------------------|
| 1. 使用执行器  | `promise.New`            | 传入 `resolve` 和 `reject` 回调执行器，适合标准异步处理       |
| 2. 带上下文取消 | `promise.NewWithContext` | 可中断的 Promise，监听 `context.Context` 超时或取消      |
| 3. 手动控制   | `promise.WithResolvers`  | 返回 `Promise` 和对应的 `resolve/reject` 函数，适合外部触发 |
| 4. 包装同步函数 | `promise.FromFunc`       | 将普通函数包装为异步 Promise，自动捕获 panic                |

示例：

```go
p := promise.New(func(resolve func(int), reject func(error)) {
  time.Sleep(1 * time.Second)
  resolve(42)
})
```

---

## 📘 使用示例

### 基础链式调用

```go
promise.Resolve(1).
    Map(func(v int) string { return fmt.Sprintf("Result: %d", v) }).
    Then(func(s string) *promise.Promise[int] {
        fmt.Println(s)
        return promise.Resolve(len(s))
    }).
    Catch(func(err error) {
        log.Println("Error:", err)
    }).
    Finally(func() {
        fmt.Println("All done")
    })
```

### 错误处理

```go
p := promise.New(func(_, reject func(error)) {
    reject(errors.New("some error"))
})
p.Catch(func(err error) {
    fmt.Println("Caught error:", err)
})
```

### 等待所有 Promise 完成

```go
p1 := promise.Resolve(1)
p2 := promise.Resolve(2)

result := promise.All(p1, p2).MustResolve()
fmt.Println(result) // [1 2]
```

---

## ⏱️ 实用工具

### 超时控制

```go
p := longRunningTask().WithTimeout(3 * time.Second)
```

### 重试机制

```go
p := promise.Retry(func() *promise.Promise[int] {
    return tryConnect()
}, promise.RetryOptions{MaxAttempts: 5, Delay: time.Second})
```

### 睡眠

```go
promise.Sleep(2 * time.Second).Then(func(_ struct{}) *promise.Promise[string] {
    return promise.Resolve("Done sleeping")
})
```

---

## 🧩 组合函数

* `All(promises...)`: 全部成功返回结果，任何一个失败立即中止
* `Any(promises...)`: 任一成功立即返回，否则聚合错误
* `AllSettled(promises...)`: 全部执行完毕，返回每个的状态
* `Race(promises...)`: 返回最先完成的 Promise（无论成功或失败）

---

## 🔄 状态控制与辅助函数

| 函数                   | 描述                   |
|----------------------|----------------------|
| `Await()`            | 阻塞直到完成，返回结果和错误       |
| `MustResolve()`      | 必须成功，否则 panic        |
| `MustReject()`       | 必须失败，否则 panic        |
| `Block()` / `Done()` | 等待所有创建的 Promise 执行完毕 |

---

## 📦 类型转换和链式映射

```go
p := promise.Resolve(3)

p.MapString(func(v int) string {
    return fmt.Sprintf("num: %d", v)
}).Then(func(s string) *promise.Promise[int] {
    return promise.Resolve(len(s))
})
```

---

## 🛡️ 错误类型

* `ErrNoResolveOrRejectCalled`
* `ErrPanic`
* `ErrAllFailed`
* `ErrTimeout`
* `ErrRetryFailed`

## 📄 License

MIT License
