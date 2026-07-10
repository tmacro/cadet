package watch

import "context"

type Watchable[T any] func(ctx context.Context, lastIndex uint64) (nextIndex uint64, value T, err error)

type Adapter[I any, O any] func(in I) (out O, err error)

func Adapt[I any, O any](w Watchable[I], adapter Adapter[I, O]) Watchable[O] {
	return func(ctx context.Context, lastIndex uint64) (nextIndex uint64, converted O, err error) {
		nI, value, err := w(ctx, lastIndex)
		if err != nil {
			return
		}

		converted, err = adapter(value)
		if err != nil {
			return
		}

		nextIndex = nI
		return
	}
}
