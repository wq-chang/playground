package pretty

type Formatter struct {
	cfg *config
}

func New(opts ...Option) *Formatter {
	cfg := newConfig(opts...)

	return &Formatter{cfg: cfg}
}

func (f *Formatter) Value(v any) string {
	return valueOpt(v, f.cfg)
}
