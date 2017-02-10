package model

import (
	"google.golang.org/appengine/datastore"
	"golang.org/x/net/context"
	"reflect"
	"fmt"
	"github.com/pkg/errors"
	"log"
)

type Query struct {
	dq *datastore.Query
	mType reflect.Type
	mValue reflect.Value
}

func NewQuery(m modelable) *Query {
	model := m.getModel();
	if !model.Registered {
		index(m);
	}

	q := datastore.NewQuery(model.structName);
	query := Query{
		dq: q,
		mType: reflect.TypeOf(m).Elem(),
	}
	log.Printf("modelable is of type %s", query.mType.Name());
	return &query;
}

/**
Filter functions
 */
func (q *Query) WithModelable(field string, ref modelable) (*Query, error) {
	refm := ref.getModel();
	if !refm.Registered {
		return nil, fmt.Errorf("Modelable reference is not registered %+v", ref);
	}

	if refm.key == nil {
		return nil, errors.New("Reference key has not been set. Can't retrieve it from datastore");
	}

	if _, ok := q.mType.FieldByName(field); !ok {
		return nil, fmt.Errorf("Struct of type %s has no field with name %s", q.mType.Name(), field);
	}

	refName := referenceName(q.mType.Name(), field);

	return q.WithField(fmt.Sprintf("%s = ", refName), refm.key), nil;
}

func (q *Query) WithField(field string, value interface{}) *Query {
	q.dq = q.dq.Filter(field, value);
	return q;
}

func (q *Query) OrderBy(fieldName string) *Query {
	q.dq = q.dq.Order(fieldName);
	return q;
}

func (q *Query) OffsetBy(offset int) *Query {
	q.dq = q.dq.Offset(offset);
	return q;
}

func (q *Query) Limit(limit int) *Query {
	q.dq = q.dq.Limit(limit);
	return q;
}

func (q *Query) Count(ctx context.Context) (int, error) {
	return q.dq.Count(ctx);
}

//Shorthand method to retrieve only the first entity satisfying the query
//It is equivalent to a Get With limit 1
func (q *Query) First(ctx context.Context, m modelable) (err error) {
	q.dq = q.dq.Limit(1);

	mm := []modelable{}

	err = q.Get(ctx, &mm);

	if err != nil {
		return err;
	}

	if len(mm) > 0 {
		src := reflect.Indirect(reflect.ValueOf(mm[0]));
		reflect.Indirect(reflect.ValueOf(m)).Set(src);
		return nil;
	}

	return datastore.ErrNoSuchEntity;
}

func (query *Query) Get(ctx context.Context, modelables *[]modelable) error {

	if (query.dq == nil) {
		return errors.New("Invalid query. Query is nil");
	}

	query.dq = query.dq.KeysOnly();

	_, e := query.get(ctx, modelables);

	if e != nil && e != datastore.Done {
		return e;
	}

	defer func() {
		query = nil;
	}()

	return nil;
}

func (query *Query) get(ctx context.Context, modelables *[]modelable) (*datastore.Cursor, error) {

	more := false;
	rc := 0;
	it := query.dq.Run(ctx);
	log.Printf("Datastore query is %+v", query.dq);

	for {

		key, err := it.Next(nil);

		if (err == datastore.Done) {
			break;
		}

		if err != nil {
			query = nil;
			return nil, err;
		}

		more = true;
		//log.Printf("RUNNING QUERY %v FOR MODEL " + data.entityName + " - FOUND ITEM WITH KEY: " + strconv.Itoa(int(key.IntID())), data.query);
		newModelable := reflect.New(query.mType);
		m, ok := newModelable.Interface().(modelable);

		if !ok {
			err = fmt.Errorf("Can't cast struct of type %s to modelable", query.mType.Name());
			query = nil;
			return nil, err
		}

		index(m);

		model := m.getModel()
		model.key = key;

		err = Read(ctx, m);
		if err != nil {
			query = nil;
			return nil, err;
		}

		*modelables = append(*modelables, m);
		rc++;
	}

	if !more {
		//if there are no more entries to be loaded, break the loop
		return nil, datastore.Done;
	} else {
		//else, if we still have entries, update cursor position
		cursor, e := it.Cursor();

		return &cursor, e;
	}
}

//retrieves up to datastore limits (currently 1000) entities from either memcache or datastore
//each datamap must have the key already set

/*func (query *Query) GetMulti(ctx context.Context) ([]Model, error) {
	//check if struct contains the fields
	const batched int = 1000;

	count, err := query.Count(ctx);

	if err != nil {
		return nil, err;
	}

	div := (count / batched);
	if (count % batched != 0) {
		div++;
	}

	log.Printf("found (count) %d items. div is %v", count, div);
	//create the blueprint
	newModelable := reflect.New(q.mType);

	//allocates memory for the resulting array
	res := make([]Model, count, count);

	var chans []chan []modelable;

	//retrieve items in concurrent batches
	mutex := new(sync.Mutex);
	for paging := 0; paging < div; paging++ {
		c := make(chan []modelable);

		go func(page int, ch chan []modelable, ctx context.Context) {

			log.Printf(ctx, "Batch #%d started", page);
			offset := page * batched;

			rq := batched;
			if page + 1 == div {
				//we need the exact number o GAE will complain since len(dst) != len(keys) in getAll
				rq = count % batched;
			}

			//copy the data query into the local copy
			//dq := datastore.NewQuery(nameOfPrototype(data.Prototype()));
			dq := query.dq;
			dq = dq.Offset(offset);
			dq = dq.KeysOnly();

			keys := make([]*datastore.Key, rq, rq);
			partial := make([]modelable, rq, rq);

			done := false;

			//Lock the loop or else other goroutine will starve the go scheduler causing a datastore timeout.
			mutex.Lock();
			c := 0;
			var cursor *datastore.Cursor;
			//we first get the keys in a batch
			for !done {

				dq = dq.Limit(200);
				//right count
				if cursor != nil {
					//since we are using start, remove the offset, or it will count from the start of the query
					dq = dq.Offset(0);
					dq = dq.Start(*cursor);
				}

				it := dq.Run(ctx);

				for {

					key, err := it.Next(nil);

					if (err == datastore.Done) {
						break;
					}

					if err != nil {
						panic(err);
					}

					dm := &dataMap{};
					*dm = *mm.dataMap;
					dm.m = reflect.New(mtype).Interface().(Prototype);

					dm.key = key;

					log.Printf(ctx, "c counter has value #%d. Max is %d, key is %s", c, rq, key.Encode());
					//populates the dst
					partial[c] = dm;
					//populate the key
					keys[c] = key;
					c++;
				}

				if c == rq {
					//if there are no more entries to be loaded, break the loop
					done = true;
					log.Debugf(data.context, "Batch #%d got %d keys from query", page, c);
				} else {
					//else, if we still have entries, update cursor position
					newCursor, e := it.Cursor();
					if e != nil {
						panic(err);
					}
					cursor = &newCursor;
				}
			}
			mutex.Unlock();

			fromCache, err := partial.cacheGetMulti(ctx, keys);

			if err != nil {
				log.Errorf(ctx, "Error retrieving multi from cache: %v", err);
			}

			c = 0;
			if len(fromCache) < rq {

				leftCount := len(keys) - len(fromCache);

				remainingKeys := make([]*datastore.Key, leftCount, leftCount);
				dst := make([]*dataMap, leftCount, leftCount);

				for i, k := range keys {
					_, ok := fromCache[k];
					if !ok {
						//add the pointer to those keys that have to be retrieved
						remainingKeys[c] = k;
						dst[c] = partial[i];
						c++;
					}
				}

				err = datastore.GetMulti(data.context, remainingKeys, dst);

				if err != nil {
					panic(err);
				}
			}

			log.Debugf(data.context, "Batch #%d retrieved all items. %d items retrieved from cache, %d items retrieved from datastore", page, len(fromCache), c);
			//now load the references of the model

			//todo: rework because it is not setting references.
			//			partial.cacheSetMulti(ctx);

			ch <- partial;

		} (paging, c, data.context);

		chans = append(chans, c);
	}

	offset := 0;
	for _ , c := range chans {
		partial := <- c;
		for j , dm := range partial {
			m := Model{dataMap: dm, searchable:mm.searchable};
			res[offset + j] = m;
		}

		offset += len(partial);
		close(c);
	}

	return res, nil;
}*/
