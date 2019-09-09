package model

import (
	"cloud.google.com/go/datastore"
	"fmt"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
	"reflect"
	"testing"
)

type Entity struct {
	Model
	Name          string
	Num           int
	Child         Child
	EmptyChild    EmptyChild `model:"zero"`
	ReadonlyChild `model:"readonly"`
	Nomo          NoModel
}

type Multiple struct {
	Num int
	Attributes []Attribute
}

type MultipleModel struct {
	Model
	Attributes []Attribute
}

type Attribute struct {
	Name string
	Value string
}

type Child struct {
	Model
	Name       string
	Grandchild Grandchild
}

type Grandchild struct {
	Model
	GrandchildNum int
}

type EmptyChild struct {
	Model
	Emptiness int
}

type ReadonlyChild struct {
	Model
	Value int
}

type NoModel struct {
	Name string
}

func TestMultiple(t *testing.T) {
	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	resetDatastoreEmulator(t)

	// create the entity
	m := Multiple{}
	m.Num = 1
	m.Attributes = make([]Attribute, 0)
	m.Attributes = append(m.Attributes, Attribute{"Color", "Red"})
	m.Attributes = append(m.Attributes, Attribute{"Color", "Blue"})

	if len(m.Attributes) != 2 {
		t.Fatalf("attributes are supposed to be 2. Found %d", len(m.Attributes))
	}

	client := ctx.Value(keyDatastoreClient).(*datastore.Client)

	newKey := datastore.IncompleteKey("Multiple", nil)
	if _, err := client.Put(ctx, newKey, &m); err != nil {
		t.Fatalf("Unable to create Multiple entity: %s", err.Error())
	}

	q := datastore.NewQuery("Multiple")
	q = q.Filter("Attributes.Value =", "Red")
	q = q.Filter("Attributes.Value =", "Blue")
	res := make([]Multiple, 0, 0)
	if _, err := client.GetAll(ctx, q, &res); err != nil {
		t.Fatalf("error retrieving multiple: %s", err.Error())
	}

	hasValue := false
	for _, v := range res {
		for _, a := range v.Attributes {
			if a.Value == "Red" {
				hasValue = true
				break
			}
		}
	}

	if !hasValue {
		t.Fatalf("Multiple has no attribute with value red. %+v", &res)
	}
}

func TestMultipleModel(t *testing.T) {
	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	resetDatastoreEmulator(t)

	// create the entity
	m := MultipleModel{}
	m.Attributes = make([]Attribute, 0)
	m.Attributes = append(m.Attributes, Attribute{"Color", "Red"})
	m.Attributes = append(m.Attributes, Attribute{"Color", "Blue"})

	if len(m.Attributes) != 2 {
		t.Fatalf("attributes are supposed to be 2. Found %d", len(m.Attributes))
	}

	if err := Create(ctx, &m); err != nil {
		t.Fatalf("Unable to create Multiple entity: %s", err.Error())
	}

	q := NewQuery((*MultipleModel)(nil))
	q.WithField("Attributes =", "red")

	res := MultipleModel{}
	if err := q.First(ctx, &res); err != nil {
		t.Fatalf("error retrieving multiple: %s", err.Error())
	}

	hasValue := false
	for _, v := range res.Attributes {
		if v.Value == "Red" {
			hasValue = true
			break
		}
	}

	if !hasValue {
		t.Fatalf("Multiple has no attribute with value red. %+v", &res)
	}
}

func TestCreateEmpty(t *testing.T) {

	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	resetDatastoreEmulator(t)

	// test correct indexing
	entity := Entity{}
	index(&entity)
	if !entity.EmptyChild.skipIfZero {
		t.Fatal("empty child is not skipIfZero")
	}

	entity.Nomo.Name = "nomo"
	entity.Name = "entity"
	entity.Child.Name = "child"
	err := Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Read(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if entity.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key")
	}

	if entity.Nomo.Name != "nomo" {
		t.Fatal("Nomodel has invalid value")
	}
}

func TestUpdate(t *testing.T) {
	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	resetDatastoreEmulator(t)

	rc := ReadonlyChild{}
	rc.Value = 1
	err := Create(ctx, &rc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// test correct indexing
	entity := Entity{}
	entity.Child.Name = "child"
	entity.Child.Grandchild.GrandchildNum = 10
	entity.ReadonlyChild = rc
	entity.ReadonlyChild.Value = 100

	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Read(ctx, &entity)
	if err != nil {
		t.Fatal(err)
	}
	entity.Child.Grandchild.GrandchildNum = 100
	entity.Child.Name = ""

	if err = Read(ctx, &entity.ReadonlyChild); err != nil {
		t.Fatal(err.Error())
	}

	if entity.ReadonlyChild.Value != 1 {
		t.Fatal("readonly child has been updated")
	}

	err = Update(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if entity.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key after update")
	}

	if entity.Child.Grandchild.GrandchildNum != 100 {
		t.Fatalf("grand child has not been updated. Num is %d", entity.Child.Grandchild.GrandchildNum)
	}

	if entity.Child.Name != "" {
		t.Fatalf("child has not been updated. Name is %s", entity.Child.Name)
	}
}

func TestDelete(t *testing.T) {

	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	resetDatastoreEmulator(t)

	rc := ReadonlyChild{}
	err := Create(ctx, &rc)
	if err != nil {
		log.Errorf(ctx, err.Error())
	}

	err = memcache.Flush(ctx)
	if err != nil {
		log.Errorf(ctx, err.Error())
	}

	// test correct indexing
	entity := Entity{}
	entity.Name = "Enzo"
	entity.Child.Name = "child"
	entity.Child.Grandchild.GrandchildNum = 10
	entity.ReadonlyChild = rc

	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Delete(ctx, &(entity.Child), &entity)
	if err != nil {
		t.Fatalf(err.Error())
	}

	q := NewQuery((*Child)(nil))
	q = q.WithField("Name = ", "child")
	err = q.First(ctx, &entity.Child)
	if err == nil {
		t.Fatalf("child must have been deleted. Found child %+v", entity.Child)
	}

	t.Logf("can't find child: %s", err.Error())

	e := Entity{}
	q = NewQuery(&e)
	q = q.WithField("Name =", "Enzo")
	err = q.First(ctx, &e)
	if err != nil {
		t.Fatalf("can't find entity with name Delete: %s", err.Error())
	}
}

const total = 100
const find = 10

// this works with strongly consistent datastore (i.e. V3), while fails with eventual consistency
func TestModel(t *testing.T) {

	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	resetDatastoreEmulator(t)

	for i := 0; i < total; i++ {
		entity := Entity{}
		entity.Name = fmt.Sprintf("%d", i)
		entity.Num = i
		entity.Child.Name = fmt.Sprintf("child-%s", entity.Name)
		entity.Child.Grandchild.GrandchildNum = i
		err := Create(ctx, &entity)
		if err != nil {
			t.Fatal(err)
		}
	}

	dst := make([]*Entity, 0)

	q := NewQuery(&Entity{})
	q = q.WithField("Num >", find)
	q = q.OrderBy("Num", ASC)
	err := q.GetMulti(ctx, &dst)
	if err != nil {
		t.Fatal(err)
	}

	many := total - find - 1
	t.Logf("looking for values with num greater than %d. There must be exactly %d entities", find, many)

	if len(dst) != many {

		last := find
		for _, e := range dst {
			if e.Num-last > 1 {
				t.Logf("gap found between %d and %d", last, e.Num)
			}
			last = e.Num
		}
		t.Fatalf("invalid number of data returned. Count is %d, it must be %d", len(dst), many)
	}

	for k := find + 1; k < total; k++ {
		idx := k - find - 1
		entity := dst[idx]
		if entity.Num != k {
			t.Fatalf("invalid error. entity at index %d has value %d.", idx, entity.Num)
		}
	}
}

func BenchmarkMapStructureLocked(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	entity := Entity{}
	typ := reflect.TypeOf(entity)
	structure := newEncodedStruct(typ.Name())
	for n := 0; n < b.N; n++ {
		mapStructureLocked(typ, structure)
	}
}

func BenchmarkIsEmpty(b *testing.B) {
	entity := Entity{}
	for n := 0; n < b.N; n++ {
		IsEmpty(&entity)
	}
}

func BenchmarkIndexing(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		entity := Entity{}
		index(&entity)
	}
}

func BenchmarkIndexingSimple(b *testing.B) {
	for n := 0; n < b.N; n++ {
		gc := Grandchild{}
		index(&gc)
	}
}

func BenchmarkReindexing(b *testing.B) {
	entity := Entity{}
	for n := 0; n < b.N; n++ {
		index(&entity)
	}
}
