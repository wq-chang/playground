package pretty

type Formatter struct {
	cfg *config
}

func New(opts ...Option) *Formatter {
	cfg := NewConfig(opts...)

	return &Formatter{cfg: cfg}
}

func (f *Formatter) Value(v any) string {
	return ValueOpt(v, f.cfg)
}
