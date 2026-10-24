package ui

// series is a fixed-size history buffer. Values are stored oldest first.
type series struct {
	limit  int
	values []float64
}

func newSeries(limit int) series {
	if limit < 1 {
		limit = 1
	}
	return series{limit: limit, values: make([]float64, 0, limit)}
}

func (s *series) add(v float64) {
	if v < 0 {
		v = 0
	}
	if len(s.values) < s.limit {
		s.values = append(s.values, v)
		return
	}
	copy(s.values, s.values[1:])
	s.values[len(s.values)-1] = v
}

func (s series) data() []float64 {
	return s.values
}

func (s series) latest() float64 {
	if len(s.values) == 0 {
		return 0
	}
	return s.values[len(s.values)-1]
}
