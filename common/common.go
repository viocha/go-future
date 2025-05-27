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

func WrapSub(err error, sub error, format string, args ...any) error {
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
