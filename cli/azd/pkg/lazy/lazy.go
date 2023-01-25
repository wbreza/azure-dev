package lazy

type InitializerFn[T comparable] func() (T, error)

// A data structure that will lazily load an instance of the underlying type
// from the specified initializer
type Lazy[T comparable] struct {
	initialized bool
	initializer InitializerFn[T]
	value       T
	error       error
}

// Creates a new Layz[T]
func NewLazy[T comparable](initializerFn InitializerFn[T]) *Lazy[T] {
	return &Lazy[T]{
		initializer: initializerFn,
	}
}

// Gets the value of the configured initializer
// Initializer will only run once on success
func (l *Lazy[T]) GetValue() (T, error) {
	if !l.initialized {
		value, err := l.initializer()
		if err == nil {
			l.value = value
			l.error = nil
			l.initialized = true
		} else {
			l.error = err
			l.initialized = false
		}
	}

	return l.value, l.error
}

// Sets a value on the lazy type
func (l *Lazy[T]) SetValue(value T) {
	l.value = value
	l.error = nil
	l.initialized = true
}
