package layer

import (
	"reflect"
	"sync"
	"time"
)

// Timing counts wire calls (including failures and pagination) and returned
// rows, or affected rows for writes. LayerMS includes transport and decoding.
type Timing struct {
	LayerMS float64 `json:"layer_ms"`
	Queries int     `json:"queries"`
	Rows    int     `json:"rows"`
	mu      sync.Mutex
}

// WithTiming gives a request its own collector while sharing the HTTP pool.
func (c *Client) WithTiming(t *Timing) *Client {
	copy := *c
	copy.timing = t
	return &copy
}

func (t *Timing) record(d time.Duration, out any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LayerMS += float64(d) / float64(time.Millisecond)
	t.Queries++
	t.Rows += responseRows(reflect.ValueOf(out))
}

func responseRows(v reflect.Value) int {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0
	}
	for _, name := range []string{"Rows", "Groups"} {
		if f := v.FieldByName(name); f.IsValid() && f.Kind() == reflect.Slice {
			return f.Len()
		}
	}
	if f := v.FieldByName("Results"); f.IsValid() && f.Kind() == reflect.Slice {
		n := 0
		for i := 0; i < f.Len(); i++ {
			n += responseRows(f.Index(i))
		}
		return n
	}
	n := 0
	for _, name := range []string{"RowsUpserted", "RowsAffected"} {
		if f := v.FieldByName(name); f.IsValid() && f.Kind() == reflect.Int {
			n = max(n, int(f.Int()))
		}
	}
	return n
}
