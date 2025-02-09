package storage

import (
	"encoding/json"
	"io"
	"iter"
	"os"
	"sync"
)

type Storage[T any] struct {
	file    *os.File
	items   map[string]T
	itemsMu sync.RWMutex
}

func Load[T any](f *os.File) (*Storage[T], error) {
	var items map[string]T

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if len(data) != 0 {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
	} else {
		items = make(map[string]T)
	}

	return &Storage[T]{
		file:  f,
		items: items,
	}, nil
}

func (s *Storage[T]) Add(key string, item T) error {
	s.itemsMu.Lock()
	defer s.itemsMu.Unlock()

	s.items[key] = item

	return s.save()
}

func (s *Storage[T]) Remove(key string) error {
	s.itemsMu.Lock()
	defer s.itemsMu.Unlock()

	delete(s.items, key)

	return s.save()
}

func (s *Storage[T]) All() iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		s.itemsMu.RLock()
		defer s.itemsMu.RUnlock()

		for k, v := range s.items {
			if !yield(k, v) {
				return
			}
		}
	}
}

func (s *Storage[T]) Close() error {
	s.itemsMu.Lock()

	defer func() {
		s.items = nil
		s.itemsMu.Unlock()
	}()

	return s.save()
}

func (s *Storage[T]) save() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}

	if err := s.file.Truncate(0); err != nil {
		return err
	}

	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}

	if _, err = s.file.Write(data); err != nil {
		return err
	}

	return nil
}
