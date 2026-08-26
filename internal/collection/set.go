package collection

type Set[T comparable] struct {
	// We are using structs as a value because empty
	// struct dose not allocate memory
	internalMap map[T]struct{}
}

func (s *Set[T]) AddAll(v ...T) {
	if s.internalMap == nil {
		s.internalMap = make(map[T]struct{})
	}
	for _, v := range v {
		s.internalMap[v] = struct{}{}
	}
}

func (s Set[T]) Slice() []T {
	if len(s.internalMap) == 0 {
		return []T{}
	}
	keys := make([]T, 0, len(s.internalMap))
	for k := range s.internalMap {
		keys = append(keys, k)
	}
	return keys
}

func (s *Set[T]) Add(v T) {
	if s.internalMap == nil {
		s.internalMap = make(map[T]struct{})
	}
	s.internalMap[v] = struct{}{}
}

func (s *Set[T]) Remove(v T) {
	if s.internalMap == nil {
		return
	}
	delete(s.internalMap, v)
}

func (s *Set[T]) Clear() {
	clear(s.internalMap)
}

func (s Set[T]) Contains(v T) bool {
	if len(s.internalMap) == 0 {
		return false
	}
	_, ok := s.internalMap[v]
	return ok
}
