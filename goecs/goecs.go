package goecs

import (
	"reflect"
)

// --- Entity ID definitions ---

// Goent is a typedef for uint64, used for entity IDs. This makes it easier
// to see what is supposed to be an entity key.
type Goent uint64

// nextEntity is a simple global counter to generate unique entity IDs.
var nextEntity Goent = 0

// CreateEntity returns a new unique entity ID.
func CreateEntity() Goent {
	id := nextEntity
	nextEntity++
	return id
}

// --- ECS core ---

const invalidIndex = -1
const alignment = 256

func nextAlignedCapacity(n int) int {
	if n%alignment == 0 {
		return n
	}
	return ((n / alignment) + 1) * alignment
}

// SparseSetInterface is a non–generic interface used for reflection-based iteration.
type SparseSetInterface interface {
	GetComponent(entity Goent) (interface{}, bool)
	GetDense() []Goent
}

// SparseSet stores a dense array of entity IDs and their corresponding component pointers.
type SparseSet[T any] struct {
	dense      []Goent
	components []*T
	sparse     []int
}

// NewSparseSet creates a new SparseSet with a default aligned capacity.
func NewSparseSet[T any]() *SparseSet[T] {
	sparse := make([]int, alignment)
	for i := range sparse {
		sparse[i] = invalidIndex
	}
	return &SparseSet[T]{
		dense:      make([]Goent, 0, alignment),
		components: make([]*T, 0, alignment),
		sparse:     sparse,
	}
}

// Emplace inserts or updates a component for an entity.
func (ss *SparseSet[T]) Emplace(entity Goent, comp T) {
	if int(entity) >= len(ss.sparse) {
		newSize := nextAlignedCapacity(int(entity) + 1)
		newSparse := make([]int, newSize)
		for i := range newSparse {
			newSparse[i] = invalidIndex
		}
		copy(newSparse, ss.sparse)
		ss.sparse = newSparse
	}

	if ss.sparse[int(entity)] != invalidIndex {
		*ss.components[ss.sparse[int(entity)]] = comp
		return
	}

	index := len(ss.dense)
	ss.dense = append(ss.dense, entity)
	ss.components = append(ss.components, &comp)
	ss.sparse[int(entity)] = index
}

// Get retrieves a pointer to the component.
func (ss *SparseSet[T]) Get(entity Goent) (*T, bool) {
	if int(entity) >= len(ss.sparse) || ss.sparse[int(entity)] == invalidIndex {
		return nil, false
	}
	return ss.components[ss.sparse[int(entity)]], true
}

// Remove deletes a component for an entity.
func (ss *SparseSet[T]) Remove(entity Goent) {
	if int(entity) >= len(ss.sparse) || ss.sparse[int(entity)] == invalidIndex {
		return
	}
	index := ss.sparse[int(entity)]
	lastIndex := len(ss.dense) - 1
	lastEntity := ss.dense[lastIndex]

	ss.dense[index] = lastEntity
	ss.components[index] = ss.components[lastIndex]
	ss.sparse[int(lastEntity)] = index

	ss.dense = ss.dense[:lastIndex]
	ss.components = ss.components[:lastIndex]
	ss.sparse[int(entity)] = invalidIndex
}

// GetComponent implements SparseSetInterface.
func (ss *SparseSet[T]) GetComponent(entity Goent) (interface{}, bool) {
	return ss.Get(entity)
}

// GetDense implements SparseSetInterface.
func (ss *SparseSet[T]) GetDense() []Goent {
	return ss.dense
}

// Registry is the central ECS registry.
type Registry struct {
	// Use reflect.Type instead of string for keys
	storages map[reflect.Type]SparseSetInterface
}

// NewRegistry creates a new ECS registry.
func NewRegistry() *Registry {
	return &Registry{storages: make(map[reflect.Type]SparseSetInterface)}
}

// typeKeyFor generates a reflection type key for a component type.
func typeKeyFor[T any]() reflect.Type {
	var zero T
	return reflect.TypeOf(zero)
}

// RegisterComponent registers a new component type. EmplaceComponent does
// this same logic if needed.
func RegisterComponent[T any](r *Registry) *SparseSet[T] {
	key := typeKeyFor[T]()
	set := NewSparseSet[T]()
	r.storages[key] = set
	return set
}

// EmplaceComponent adds or replaces a component by entity id.
func EmplaceComponent[T any](r *Registry, entity Goent, comp T) {
	key := typeKeyFor[T]()
	storageInterface, exists := r.storages[key]
	if !exists {
		storageInterface = NewSparseSet[T]()
		r.storages[key] = storageInterface
	}
	storage := storageInterface.(*SparseSet[T])
	storage.Emplace(entity, comp)
}

// GetComponent retrieves a pointer to a component.
func GetComponent[T any](r *Registry, entity Goent) (*T, bool) {
	key := typeKeyFor[T]()
	storageInterface, exists := r.storages[key]
	if !exists {
		return nil, false
	}
	storage := storageInterface.(*SparseSet[T])
	return storage.Get(entity)
}

// RemoveComponent removes a component by entity id.
func RemoveComponent[T any](r *Registry, entity Goent) {
	key := typeKeyFor[T]()
	if storageInterface, exists := r.storages[key]; exists {
		storage := storageInterface.(*SparseSet[T])
		storage.Remove(entity)
	}
}

// IterateReflective uses reflection for iteration. It is much slower but flexible.
func (r *Registry) IterateReflective(f interface{}) {
	fVal := reflect.ValueOf(f)
	fType := fVal.Type()

	// Validate signature: first arg must be Goent, rest are *SomeComponent
	if fType.Kind() != reflect.Func || fType.NumIn() < 1 || fType.In(0) != reflect.TypeOf(Goent(0)) {
		panic("Iterate requires a function (entity Goent, *T1, *T2, ...)")
	}
	compCount := fType.NumIn() - 1
	if compCount == 0 {
		panic("Iterate function must have at least one component parameter")
	}

	// Figure out storages for each parameter
	storages := make([]SparseSetInterface, compCount)
	for i := 0; i < compCount; i++ {
		paramType := fType.In(i + 1)
		if paramType.Kind() == reflect.Ptr {
			paramType = paramType.Elem()
		}
		storage, exists := r.storages[paramType]
		if !exists {
			// If any storage is missing, there's nothing to iterate
			return
		}
		storages[i] = storage
	}

	// Pick the smallest dense array to drive iteration
	baseIndex := 0
	minLen := len(storages[0].GetDense())
	for i := 1; i < compCount; i++ {
		l := len(storages[i].GetDense())
		if l < minLen {
			baseIndex = i
			minLen = l
		}
	}
	baseDense := storages[baseIndex].GetDense()

	// Pre-allocate the call arguments
	args := make([]reflect.Value, compCount+1)

	// Iterate over entities in the base storage
	for _, entity := range baseDense {
		args[0] = reflect.ValueOf(entity)
		valid := true

		for i, storage := range storages {
			comp, ok := storage.GetComponent(entity)
			if !ok {
				valid = false
				break
			}
			// comp is already a pointer to T
			args[i+1] = reflect.ValueOf(comp)
		}

		if valid {
			fVal.Call(args)
		}
	}
}

// --- Typed (non-reflective) iteration helpers ---
// Goes up to 4 supported arguments. For more, consider codegen or a better pattern.

// iterateDense is a helper to loop over a dense slice.
func iterateDense(dense []Goent, f func(entity Goent)) {
	for _, e := range dense {
		f(e)
	}
}

// getStorage returns the typed storage for a component type from the registry.
func getStorage[T any](r *Registry) *SparseSet[T] {
	key := typeKeyFor[T]()
	storageInterface, exists := r.storages[key]
	if !exists {
		return nil
	}
	return storageInterface.(*SparseSet[T])
}

// Iterate2 iterates over entities that have both T1 and T2 components.
func Iterate2[T1 any, T2 any](r *Registry, f func(entity Goent, c1 *T1, c2 *T2)) {
	s1 := getStorage[T1](r)
	s2 := getStorage[T2](r)
	if s1 == nil || s2 == nil {
		return
	}

	// Decide which dense array is smaller
	baseDense := s1.dense
	if len(s2.dense) < len(baseDense) {
		baseDense = s2.dense
	}

	iterateDense(baseDense, func(entity Goent) {
		c1, ok1 := s1.Get(entity)
		c2, ok2 := s2.Get(entity)
		if ok1 && ok2 {
			f(entity, c1, c2)
		}
	})
}

// Iterate3 iterates over entities that have T1, T2, and T3 components.
func Iterate3[T1 any, T2 any, T3 any](r *Registry, f func(entity Goent, c1 *T1, c2 *T2, c3 *T3)) {
	s1 := getStorage[T1](r)
	s2 := getStorage[T2](r)
	s3 := getStorage[T3](r)
	if s1 == nil || s2 == nil || s3 == nil {
		return
	}

	// Decide which dense array is smaller
	baseDense := s1.dense
	if len(s2.dense) < len(baseDense) {
		baseDense = s2.dense
	}
	if len(s3.dense) < len(baseDense) {
		baseDense = s3.dense
	}

	iterateDense(baseDense, func(entity Goent) {
		c1, ok1 := s1.Get(entity)
		c2, ok2 := s2.Get(entity)
		c3, ok3 := s3.Get(entity)
		if ok1 && ok2 && ok3 {
			f(entity, c1, c2, c3)
		}
	})
}

// Iterate4 iterates over entities that have T1, T2, T3, and T4 components.
func Iterate4[T1 any, T2 any, T3 any, T4 any](r *Registry, f func(entity Goent, c1 *T1, c2 *T2, c3 *T3, c4 *T4)) {
	s1 := getStorage[T1](r)
	s2 := getStorage[T2](r)
	s3 := getStorage[T3](r)
	s4 := getStorage[T4](r)
	if s1 == nil || s2 == nil || s3 == nil || s4 == nil {
		return
	}

	// Decide which dense array is smaller
	baseDense := s1.dense
	if len(s2.dense) < len(baseDense) {
		baseDense = s2.dense
	}
	if len(s3.dense) < len(baseDense) {
		baseDense = s3.dense
	}
	if len(s4.dense) < len(baseDense) {
		baseDense = s4.dense
	}

	iterateDense(baseDense, func(entity Goent) {
		c1, ok1 := s1.Get(entity)
		c2, ok2 := s2.Get(entity)
		c3, ok3 := s3.Get(entity)
		c4, ok4 := s4.Get(entity)
		if ok1 && ok2 && ok3 && ok4 {
			f(entity, c1, c2, c3, c4)
		}
	})
}
