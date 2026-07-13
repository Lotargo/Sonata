package httpapi

type OutputFilter interface {
	Push(string) (string, error)
	Close() (string, error)
}

type OutputFilterFactory func() OutputFilter

type passthroughOutputFilter struct{}

func (passthroughOutputFilter) Push(value string) (string, error) { return value, nil }
func (passthroughOutputFilter) Close() (string, error)            { return "", nil }

func newOutputFilter(factory OutputFilterFactory) OutputFilter {
	if factory == nil {
		return passthroughOutputFilter{}
	}
	filter := factory()
	if filter == nil {
		return passthroughOutputFilter{}
	}
	return filter
}
