package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type IndexOption struct {
	Settings map[string]any
	Mappings map[string]any
}

/*
打开/关闭索引
获取索引设置
更新索引设置
获取索引映射
更新索引映射
刷新索引
强制合并索引
复制索引（Reindex）
收缩索引（Shrink）
拆分索引（Split）
获取索引统计信息
*/

// Create creates a new Elasticsearch index with the specified settings and mappings.
//
// Parameters:
//   - indexName: The name of the index to create.
//   - settings: Optional settings for the index. Can be nil if no custom settings are needed.
//   - mappings: Optional mappings for the index. Can be nil if no custom mappings are needed.
//
// Returns:
//   - error: An error if one occurred, otherwise nil.
//
// Description:
//
//	This function creates a new Elasticsearch index with the given name, settings, and mappings.
//	If the index already exists, it will return an error.
//
// Example:
//
//	settings := map[string]interface{}{
//	    "number_of_shards": 3,
//	    "number_of_replicas": 2,
//	}
//	mappings := map[string]interface{}{
//	    "properties": map[string]interface{}{
//	        "title": map[string]interface{}{
//	            "type": "text",
//	        },
//	        "content": map[string]interface{}{
//	            "type": "text",
//	        },
//	        "date": map[string]interface{}{
//	            "type": "date",
//	        },
//	    },
//	}
//	err := Create("my_index", settings, mappings)
//	if err != nil {
//	    log.Fatal(err)
//	}
func (*index) Create(indexName string, options ...*IndexOption) error {
	// Create the index body
	body := make(map[string]any)
	if len(options) > 0 {
		if options[0] != nil {
			if options[0].Settings != nil {
				body["settings"] = options[0].Settings
			}
			if options[0].Mappings != nil {
				body["mappings"] = options[0].Mappings
			}
		}
	}
	// if settings != nil {
	// 	body["settings"] = settings
	// }
	// if mappings != nil {
	// 	body["mappings"] = mappings
	// }

	// Convert the body to JSON
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("error marshaling index body: %w", err)
	}

	// Create the index
	res, err := client.Indices.Create(
		indexName,
		client.Indices.Create.WithBody(bytes.NewReader(bodyJSON)),
		client.Indices.Create.WithContext(context.Background()),
	)
	if err != nil {
		return fmt.Errorf("error creating index: %w", err)
	}
	defer res.Body.Close()

	// Check the response
	if res.IsError() {
		return fmt.Errorf("error creating index: %s", res.String())
	}

	return nil
}

// Exists checks if an Elasticsearch index exists.
//
// Parameters:
//   - indexName: The name of the index to check.
//
// Returns:
//   - bool: true if the index exists, false otherwise.
//   - error: An error if one occurred, otherwise nil.
//
// Description:
//
//	This function checks whether an Elasticsearch index with the given name exists.
//
// Example:
//
//	exists, err := Exists("my_index")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if exists {
//	    fmt.Println("Index exists")
//	} else {
//	    fmt.Println("Index does not exist")
//	}
func (*index) Exists(indexName string) (bool, error) {
	res, err := client.Indices.Exists([]string{indexName})
	if err != nil {
		return false, fmt.Errorf("error checking index existence: %w", err)
	}
	defer res.Body.Close()

	return res.StatusCode == http.StatusOK, nil
}

// Delete removes an Elasticsearch index.
//
// Parameters:
//   - indexName: The name of the index to delete.
//
// Returns:
//   - error: An error if one occurred, otherwise nil.
//
// Description:
//
//	This function deletes an Elasticsearch index with the given name.
//	If the index doesn't exist, it will return an error.
//
// Example:
//
//	err := Delete("my_index")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (*index) Delete(indexName string) error {
	// Delete the index
	res, err := client.Indices.Delete([]string{indexName})
	if err != nil {
		return fmt.Errorf("error deleting index: %w", err)
	}
	defer res.Body.Close()

	// Check the response
	if res.IsError() {
		return fmt.Errorf("error deleting index: %s", res.String())
	}

	return nil
}
