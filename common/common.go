package common

import (
	"errors"
	"fmt"
	"sync"
)

func WrapMsg(err error, format string, args ...any) error {
	return errors.Join(
		err,
		fmt.Errorf(format, args...),
	)
}

func WrapSub(sub error, err error, format string, args ...any) error {
	return errors.Join(
		WrapMsg(err, format, args...),
		sub,
	)
}

func ParseOptional[T any](v []T, defaultVal T) T {
	if len(v) == 0 {
		return defaultVal
	}
	return v[0]
}

func GetWgChan(wg *sync.WaitGroup) chan struct{} {
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}

// 如果传入的值是 error 类型，则直接返回该 error，否则将其包装为 error 类型
func ToError(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return fmt.Errorf("%v", r)
}

// 可以从panic中安全地执行函数，返回是否成功执行
func DoSafe(f func()) error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = ToError(r)
			}
		}()
		f()
	}()
	return err
}
