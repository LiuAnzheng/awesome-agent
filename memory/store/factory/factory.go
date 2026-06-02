package factory

import (
	"fmt"

	"github.com/LiuAnzheng/memoria/memory/store"
)

type StructuredStoreCtor func(opts map[string]interface{}) (store.StructuredStore, error)

type VectorStoreCtor func(opts map[string]interface{}) (store.VectorStore, error)

type GraphStoreCtor func(opts map[string]interface{}) (store.GraphStore, error)

type EmbeddingServiceCtor func(opts map[string]interface{}) (store.EmbeddingService, error)

var (
	structuredStoreMap  map[string]StructuredStoreCtor  = map[string]StructuredStoreCtor{}
	vectorStoreMap      map[string]VectorStoreCtor      = map[string]VectorStoreCtor{}
	graphStoreMap       map[string]GraphStoreCtor       = map[string]GraphStoreCtor{}
	embeddingServiceMap map[string]EmbeddingServiceCtor = map[string]EmbeddingServiceCtor{}
)

func RegisterStructuredStore(driver string, ctor StructuredStoreCtor) {
	structuredStoreMap[driver] = ctor
}

func NewStructuredStore(driver string, opts map[string]interface{}) (store.StructuredStore, error) {
	ctor, ok := structuredStoreMap[driver]
	if !ok {
		return nil, fmt.Errorf("driver %s not registered", driver)
	}
	return ctor(opts)
}

func RegisterVectorStore(driver string, ctor VectorStoreCtor) {
	vectorStoreMap[driver] = ctor
}

func NewVectorStore(driver string, opts map[string]interface{}) (store.VectorStore, error) {
	ctor, ok := vectorStoreMap[driver]
	if !ok {
		return nil, fmt.Errorf("driver %s not registered", driver)
	}
	return ctor(opts)
}

func RegisterGraphStore(driver string, ctor GraphStoreCtor) {
	graphStoreMap[driver] = ctor
}

func NewGraphStore(driver string, opts map[string]interface{}) (store.GraphStore, error) {
	ctor, ok := graphStoreMap[driver]
	if !ok {
		return nil, fmt.Errorf("driver %s not registered", driver)
	}
	return ctor(opts)
}

func RegisterEmbeddingService(driver string, ctor EmbeddingServiceCtor) {
	embeddingServiceMap[driver] = ctor
}

func NewEmbeddingService(driver string, opts map[string]interface{}) (store.EmbeddingService, error) {
	ctor, ok := embeddingServiceMap[driver]
	if !ok {
		return nil, fmt.Errorf("driver %s not registered", driver)
	}
	return ctor(opts)
}
