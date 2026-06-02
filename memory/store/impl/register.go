package impl

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiuAnzheng/memoria/memory/store"
	"github.com/LiuAnzheng/memoria/memory/store/factory"
)

// 注册默认构造函数
func init() {
	factory.RegisterStructuredStore("sqlite", NewSQLiteStore)
	factory.RegisterVectorStore("qdrant", NewQdrantStore)
	factory.RegisterGraphStore("neo4j", NewNeo4jStore)
	factory.RegisterEmbeddingService("openai", NewOpenAIEmbedding)
}

var NewSQLiteStore factory.StructuredStoreCtor = func(opts map[string]interface{}) (store.StructuredStore, error) {
	dbPath := "./data/memory.db"
	if v, ok := opts["db_path"]; ok {
		if s, ok := v.(string); ok && s != "" {
			dbPath = s
		}
	}
	return &SQLiteStore{dbPath: dbPath}, nil
}

var NewQdrantStore factory.VectorStoreCtor = func(opts map[string]interface{}) (store.VectorStore, error) {
	protocol := "http"
	host := "127.0.0.1"
	port := 6333
	apiKey := ""

	if v, ok := opts["protocol"]; ok {
		if s, ok := v.(string); ok && s != "" {
			protocol = s
		}
	}

	if v, ok := opts["host"]; ok {
		if s, ok := v.(string); ok && s != "" {
			host = s
		}
	}
	if v, ok := opts["port"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				port = n
			}
		case float64:
			if n > 0 {
				port = int(n)
			}
		}
	}
	if v, ok := opts["api_key"]; ok {
		if s, ok := v.(string); ok {
			apiKey = s
		}
	}

	return &QdrantStore{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: fmt.Sprintf("%s://%s:%d", protocol, host, port),
	}, nil
}

var NewNeo4jStore factory.GraphStoreCtor = func(opts map[string]interface{}) (store.GraphStore, error) {
	url := "http://127.0.0.1:7474"
	dbName := "neo4j"
	username := "neo4j"
	password := "neo4j"

	if v, ok := opts["url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			url = s
		}
	}
	if v, ok := opts["db"]; ok {
		if s, ok := v.(string); ok && s != "" {
			dbName = s
		}
	}
	if v, ok := opts["username"]; ok {
		if s, ok := v.(string); ok && s != "" {
			username = s
		}
	}
	if v, ok := opts["password"]; ok {
		if s1, ok := v.(string); ok {
			password = s1
		}
		if s2, ok := v.(int); ok {
			password = strconv.Itoa(s2)
		}
	}

	return &Neo4jStore{
		client: &http.Client{Timeout: 30 * time.Second},
		txURL:  fmt.Sprintf("%s/db/%s/tx/commit", strings.TrimRight(url, "/"), dbName),
		auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)),
	}, nil
}

var NewOpenAIEmbedding factory.EmbeddingServiceCtor = func(opts map[string]interface{}) (store.EmbeddingService, error) {
	e := &OpenAIEmbedding{
		modelID:   "text-embedding-3-small",
		baseURL:   "https://api.openai.com/",
		dimension: 1024,
		batchSize: 32,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
	if v, ok := opts["model_id"]; ok {
		if s, ok := v.(string); ok && s != "" {
			e.modelID = s
		}
	}
	if v, ok := opts["api_key"]; ok {
		if s, ok := v.(string); ok {
			e.apiKey = s
		}
	}
	if v, ok := opts["base_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			e.baseURL = s
		}
	}
	if v, ok := opts["dimension"]; ok {
		switch n := v.(type) {
		case uint64:
			if n > 0 {
				e.dimension = n
			}
		case int:
			if n > 0 {
				e.dimension = uint64(n)
			}
		case int64:
			if n > 0 {
				e.dimension = uint64(n)
			}
		case float64:
			if n > 0 {
				e.dimension = uint64(n)
			}
		}
	}
	if v, ok := opts["batch_size"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				e.batchSize = n
			}
		case float64:
			if n > 0 {
				e.batchSize = int(n)
			}
		}
	}
	return e, nil
}
