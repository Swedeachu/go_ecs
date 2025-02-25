package goecs

import (
	"fmt"
	"math/rand"
	"time"
)

// --- Example Components ---

type testTransform struct {
	X, Y, Z float64
}

type testRigidBody struct {
	Vx, Vy, Vz float64
}

type testMesh struct {
	ID int
}

type testMaterial struct {
	ID int
}

type testBehavior struct {
	Active bool
}

// -- Actual test code --

// TestECS runs all ECS test cases
func TestECS() {
	const numEntities = 10000
	reg := NewRegistry()

	fmt.Printf("Starting ECS tests...\n\n")

	measureTime("Entity Allocation and Component Emplacement", func() {
		TestEmplaceComponents(reg, numEntities)
	})

	measureTime("Get and Modify Component", func() {
		TestModifyComponent(reg, numEntities)
	})

	measureTime("Component Iteration", func() {
		TestComponentIteration(reg)
	})

	measureTime("Reflective Component Iteration", func() {
		TestIterateReflective(reg)
	})

	measureTime("Random Component Removal", func() {
		TestRandomRemovals(reg, numEntities)
	})
}

// measureTime runs a test function and prints its execution time
func measureTime(name string, fn func()) {
	start := time.Now()
	fn()
	elapsed := time.Since(start)

	// Choose appropriate formatting based on the duration
	if elapsed < time.Microsecond {
		fmt.Printf("%s took %d ns\n\n", name, elapsed.Nanoseconds())
	} else if elapsed < time.Millisecond {
		fmt.Printf("%s took %d µs\n\n", name, elapsed.Microseconds())
	} else {
		fmt.Printf("%s took %v\n\n", name, elapsed)
	}
}

// TestEmplaceComponents creates entities and assigns components
func TestEmplaceComponents(reg *Registry, numEntities int) {
	for i := 0; i < numEntities; i++ {
		id := CreateEntity()
		EmplaceComponent(reg, id, testTransform{
			X: float64(i),
			Y: float64(i) * 2,
			Z: float64(i) * 3,
		})
		EmplaceComponent(reg, id, testRigidBody{
			Vx: float64(i) * 0.1,
			Vy: float64(i) * 0.2,
			Vz: float64(i) * 0.3,
		})
		if i%2 == 0 {
			EmplaceComponent(reg, id, testMesh{ID: i})
			EmplaceComponent(reg, id, testMaterial{ID: i})
		}
		if i%3 == 0 {
			EmplaceComponent(reg, id, testBehavior{Active: true})
		}
	}
}

// TestComponentIteration iterates over entities with Transform and RigiBody components
func TestComponentIteration(reg *Registry) {
	count := 0
	Iterate2(reg, func(entity Goent, t *testTransform, rb *testRigidBody) {
		t.X += rb.Vx
		t.Y += rb.Vy
		t.Z += rb.Vz
		count++
	})
	fmt.Printf("Iterated over %d entities with Transform and RigiBody components.\n", count)
}

// TestIterateReflective tests the reflection-based iteration over 4 components.
func TestIterateReflective(reg *Registry) {
	count := 0

	reg.IterateReflective(func(entity Goent, t *testTransform, rb *testRigidBody, m *testMesh, mat *testMaterial) {
		// Modify some of the components just to do something
		t.X += rb.Vx
		t.Y += rb.Vy
		t.Z += rb.Vz

		m.ID += 10

		count++
	})

	fmt.Printf("Reflective iteration processed %d entities with Transform, RigiBody, Mesh, and Material components.\n", count)
}

// TestRandomRemovals removes random components from entities
func TestRandomRemovals(reg *Registry, numEntities int) {
	count := 0
	for i := 0; i < numEntities/10; i++ {
		count++
		entity := Goent(rand.Uint64() % uint64(numEntities))
		RemoveComponent[testTransform](reg, entity)
		RemoveComponent[testRigidBody](reg, entity)
		RemoveComponent[testMesh](reg, entity)
		RemoveComponent[testMaterial](reg, entity)
		RemoveComponent[testBehavior](reg, entity)
	}
	fmt.Printf("Random removals complete with %d entities removed\n", count)
}

// TestModifyComponent retrieves, modifies, and verifies component changes
func TestModifyComponent(reg *Registry, numEntities int) {
	entity := Goent(rand.Uint64() % uint64(numEntities))
	if t, ok := GetComponent[testTransform](reg, entity); ok {
		fmt.Printf("Before modification: Entity %d Transform: %+v\n", entity, *t)

		// Modify the component
		t.X += 10000
		t.Y += 20000
		t.Z += 30000

		// Retrieve again to verify the change
		if updatedT, ok := GetComponent[testTransform](reg, entity); ok {
			fmt.Printf("After modification (should be different): Entity %d Transform: %+v\n", entity, *updatedT)
		} else {
			fmt.Println("Failed to retrieve Transform after modification.")
		}
	} else {
		fmt.Printf("Entity %d does not have a Transform component.\n", entity)
	}
}
