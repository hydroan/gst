## Create

> `Request`
>
> ```bash
> curl --silent --location --request POST 'http://localhost:8080/api/user' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
>    "id": "user01",
>    "name": "user01",
>    "email": "user01@gmail.com"
> }'
> ```
>
> `Response`
>
> ```json
> {
>   "code": 0,
>   "data": {
>     "id": "user01",
>     "created_at": "2024-12-25T12:43:21.250766+08:00",
>     "updated_at": "2024-12-25T12:43:21.241+08:00",
>     "name": "user01",
>     "email": "user01@gmail.com"
>   },
>   "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> user.ID = "user01"
> user.Name = "user01"
> user.Email = "user01@gmail.com"
> database.Database[*model.User]().Create(user)
> ```

## Delete

### Delete one resource (by route parameter)

> `Request`
>
> ```bash
> # Delete user whose id is 'user01'
> curl --silent --location --request DELETE 'http://localhost:8080/api/user/user01' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": "",
>     "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> user.SetID("user01")
> database.Database[*model.User]().Delete(user)
> ```

### Delete one resource (by http body)

> `Rrequest`
>
> ```bash
> # Delete user whose id is 'user01'.
> curl --silent --location --request DELETE 'http://localhost:8080/api/user' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data '[
>    "user01"
> ]'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": "",
>     "msg": "success"
> }
> ```
>
> `Database Equivalent`
>
> ```go
> user := new(model.User)
> user.SetID("user01")
> database.Database[*model.User]().Delete(user)
> ```

### Delete multiple resources (by http body)

> `Request`
>
> ```bash
> # Delete user whose id are 'user01', 'user02'.
> curl --silent --location --request DELETE 'http://localhost:8080/api/user' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data '[
>    "user01", "user02"
> ]'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": "",
>     "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> u1, u2 := new(model.User), new(model.User)
> u1.SetID("user01")
> u2.SetID("user02")
> database.Database[*model.User]().Delete(u1, u2)
> ```

### Delete multiple resource (by route parameter and http body)

> `Request`
>
> ```bash
> # Delete user whose id are 'user01', 'user02', "user03".
> curl --silent --location --request DELETE 'http://localhost:8080/api/user/user01' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data '[
> "user02", "user03"
> ]'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": "",
>     "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> u1, u2, u3 := new(model.User), new(model.User), new(model.User)
> u1.SetID("user01")
> u2.SetID("user02")
> u3.SetID("user03")
> database.Database[*model.User]().Delete(u1, u2, u3)
> ```

## Update

> `Request`
>
> ```bash
> curl --silent --location --request PUT 'http://localhost:8080/api/user' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
>    "id": "user01",
>    "name": "user01_modifed",
>    "email": "user01_modifed@gmail.com"
> }'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": {
>        "id": "user01",
>        "created_at": "2024-12-25T13:01:01.634+08:00",
>        "updated_at": "2024-12-25T13:26:18.307+08:00",
>        "name": "user01_modifed",
>        "email": "user01_modifed@gmail.com"
>     },
>     "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> user.ID = "user01"
> user.Name = "user01_modifed"
> user.Email = "user01_modifed@gmail.com"
> database.Database[*model.User]().Update(user)
> ```

## UpdatePartial

### Update partial by router parameter

> `Request`
>
> ```bash
> curl --silent --location --request PATCH 'http://localhost:8080/api/user/user01' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
>    "name": "user01_modified",
>    "email": "user01_modifed@gmail.com"
> }'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": {
>        "id": "user01",
>        "created_at": "2024-12-25T17:22:28.558+08:00",
>        "updated_at": "2024-12-26T10:29:32.837+08:00",
>        "name": "user01_modified",
>        "email": "user01_modifed@gmail.com",
>        "avatar": "https://myavataor.com",
>        "sunname": "mysunname",
>        "nickname": "mynickname"
>     },
>     "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().UpdateById("user01", "name", "user01_modified")
> database.Database[*model.User]().UpdateById("user01", "email", "user01_modified@gmail.com")
> ```

### Update partial by http body

> `Request`
>
> ```bash
> curl --silent --location --request PATCH 'http://localhost:8080/api/user' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
>     "id": "user01",
>     "name": "user01_modified2",
>     "email": "user01_modifed2@gmail.com"
> }'
> ```
>
> `Response`
>
> ```json
> {
>     "code": 0,
>     "data": {
>        "id": "user01",
>        "created_at": "2024-12-25T17:22:28.558+08:00",
>        "updated_at": "2024-12-26T10:30:28.484+08:00",
>        "name": "user01_modified2",
>        "email": "user01_modifed2@gmail.com",
>        "avatar": "https://myavataor.com",
>        "sunname": "mysunname",
>        "nickname": "mynickname"
>     },
>     "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().UpdateById("user01", "name", "user01_modified")
> database.Database[*model.User]().UpdateById("user01", "email", "user01_modified@gmail.com")
> ```

## List

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
>   "code": 0,
>   "data": {
>     "items": [
>       {
>         "id": "user01",
>         "created_at": "2024-12-25T11:21:21.134+08:00",
>         "updated_at": "2024-12-25T11:21:21.135+08:00",
>         "name": "user01",
>         "email": "user01@gmail.com"
>       },
>       {
>         "id": "user02",
>         "created_at": "2024-12-25T11:23:18.017+08:00",
>         "updated_at": "2024-12-25T11:23:18.017+08:00",
>         "name": "user02",
>         "email": "user02@gmail.com"
>       }
>     ],
>     "total": 2
>   },
>   "msg": "success"
> }
> ```

### Query Parameters

#### `page=number`, `size=number` for paginatin.

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?page=1&size=10' \
> --header 'Authorization: Bearer -'
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().WithPagination(1, 20).List(&users)
> ```

#### `_expand=parent,children`

NOTE: set value to `all` to expand all field.

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/category/fruit?_expand=all' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "id": "fruit",
>  "created_at": "2024-12-25T15:36:25.156+08:00",
>  "updated_at": "2024-12-25T15:36:25.156+08:00",
>  "name": "fruit",
>  "status": 1,
>  "parent_id": "root",
>  "children": [
>    {
>      "id": "apple",
>      "created_at": "2024-12-25T15:36:25.156+08:00",
>      "updated_at": "2024-12-25T15:36:25.156+08:00",
>      "name": "apple",
>      "status": 1,
>      "parent_id": "fruit"
>    },
>    {
>      "id": "banana",
>      "created_at": "2024-12-25T15:36:25.156+08:00",
>      "updated_at": "2024-12-25T15:36:25.156+08:00",
>      "name": "banana",
>      "status": 1,
>      "parent_id": "fruit"
>    }
>  ],
>  "parent": {
>    "id": "root",
>    "created_at": "2024-12-25T15:36:25.156+08:00",
>    "updated_at": "2024-12-25T15:36:25.156+08:00",
>    "name": "root",
>    "status": 0,
>    "parent_id": "root"
>  }
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> categories := make([]*model.Category, 0)
> database.Database[*model.Category]().WithExpand(new(model.Category).Expands()).List(&categories)
> ```

#### `_expand=children`

```go
database.Database[*model.Category]().WithExpand(new(model.Category).Expands()).List(&categories)
```

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/category/fruit?_expand=children' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "id": "fruit",
>  "created_at": "2024-12-25T15:36:25.156+08:00",
>  "updated_at": "2024-12-25T15:36:25.156+08:00",
>  "name": "fruit",
>  "status": 1,
>  "parent_id": "root",
>  "children": [
>    {
>      "id": "apple",
>      "created_at": "2024-12-25T15:36:25.156+08:00",
>      "updated_at": "2024-12-25T15:36:25.156+08:00",
>      "name": "apple",
>      "status": 1,
>      "parent_id": "fruit"
>    },
>    {
>      "id": "banana",
>      "created_at": "2024-12-25T15:36:25.156+08:00",
>      "updated_at": "2024-12-25T15:36:25.156+08:00",
>      "name": "banana",
>      "status": 1,
>      "parent_id": "fruit"
>    }
>  ]
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> categories := make([]*model.Category, 0)
> database.Database[*model.Category]().WithExpand([]string{"Children"}).List(&categories)
> ```
>
> 

#### `_expand=children`,`_depth=3`

\_depth specify the depth to expand.

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/category/fruit?_expand=children&_depth=3' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "id": "fruit",
>  "created_at": "2024-12-25T15:36:25.156+08:00",
>  "updated_at": "2024-12-25T15:36:25.156+08:00",
>  "name": "fruit",
>  "status": 1,
>  "parent_id": "root",
>  "children": [
>    {
>      "id": "apple",
>      "created_at": "2024-12-25T15:36:25.156+08:00",
>      "updated_at": "2024-12-25T15:36:25.156+08:00",
>      "name": "apple",
>      "status": 1,
>      "parent_id": "fruit",
>      "children": [
>        {
>          "id": "apple1",
>          "created_at": "2024-12-25T15:36:25.156+08:00",
>          "updated_at": "2024-12-25T15:36:25.156+08:00",
>          "name": "apple1",
>          "status": 1,
>          "parent_id": "apple"
>        },
>        {
>          "id": "apple2",
>          "created_at": "2024-12-25T15:36:25.156+08:00",
>          "updated_at": "2024-12-25T15:36:25.156+08:00",
>          "name": "apple2",
>          "status": 1,
>          "parent_id": "apple"
>        }
>      ]
>    },
>    {
>      "id": "banana",
>      "created_at": "2024-12-25T15:36:25.156+08:00",
>      "updated_at": "2024-12-25T15:36:25.156+08:00",
>      "name": "banana",
>      "status": 1,
>      "parent_id": "fruit",
>      "children": [
>        {
>          "id": "banana1",
>          "created_at": "2024-12-25T15:36:25.156+08:00",
>          "updated_at": "2024-12-25T15:36:25.156+08:00",
>          "name": "banana1",
>          "status": 1,
>          "parent_id": "banana"
>        },
>        {
>          "id": "banana2",
>          "created_at": "2024-12-25T15:36:25.156+08:00",
>          "updated_at": "2024-12-25T15:36:25.156+08:00",
>          "name": "banana2",
>          "status": 1,
>          "parent_id": "banana"
>        }
>      ]
>    }
>  ]
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> categories := make([]*model.Category, 0)
> database.Database[*model.Category]().WithExpand([]string{"Children.Children.Children"}).List(&categories)
> ```

#### `field_name=value`, `_fuzzy_true`

NOTE: you should always make sure the model field has `schema` tag. for example:

```golang
type User struct {
	model.Base

	Name     *string `json:"name,omitempty" schema:"name"`
	Email    *string `json:"email,omitempty" schema:"email"`
	Avatar   *string `json:"avatar,omitempty" schema:"avatar"`
	Sunname  *string `json:"sunname,omitempty" schema:"sunname"`
	Nickname *string `json:"nickname,omitempty" schema:"nickname"`
}
```

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```bash
> {
>  "code": 0,
>  "data": {
>      "items": [
>          {
>              "id": "user01",
>              "created_at": "2024-12-25T17:22:28.558+08:00",
>              "updated_at": "2024-12-25T17:22:28.558+08:00",
>              "name": "user01",
>              "email": "user01@gmail.com"
>          }
>      ],
>      "total": 1
>  },
>  "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().WithQuery(&model.User{Name: util.ValueOf("user01")}).List(&users)
> ```

> `request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_fuzzy=true' \
> --header 'Authorization: Bearer -'
> ```
>
> `response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "items": [
>    {
>      "id": "user01",
>      "created_at": "2024-12-25T17:22:28.558+08:00",
>      "updated_at": "2024-12-25T17:22:28.558+08:00",
>      "name": "user01",
>      "email": "user01@gmail.com"
>    },
>    {
>      "id": "user01_deferred_id",
>      "created_at": "2024-12-25T17:21:03.836+08:00",
>      "updated_at": "2024-12-25T17:21:03.836+08:00",
>      "name": "user01_deferred_id",
>      "email": "user01@example.com"
>    },
>    {
>      "id": "user01_with_id",
>      "created_at": "2024-12-25T17:21:03.834+08:00",
>      "updated_at": "2024-12-25T17:21:03.834+08:00",
>      "name": "user01_with_id",
>      "email": "user01@example.com"
>    }
>  ],
>  "total": 3
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().WithQuery(&model.User{Name: util.ValueOf("user01")}, true).List(&users)
> ```

#### `_sortby=xxx`

>   `Request`
>
>   ```bash
>   # _sortby=name
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_fuzzy=true&_sortby=name' \
>   --header 'Authorization: Bearer -'
>   
>   # _sortby=name desc
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_fuzzy=true&_sortby=name%20desc' \
>   --header 'Authorization: Bearer -'
>   
>   # _sortby=name desc, created_at
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_fuzzy=true&_sortby=name%20desc%2C%20created_at' \
>   --header 'Authorization: Bearer -'
>   
>   # _sortby=name desc, created_at asc
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_fuzzy=true&_sortby=name%20desc%2C%20created_at%20asc' \
>   --header 'Authorization: Bearer -'
>   ```
>
>   `Database equivalent`
>
>   ```go
>   database.Database[*model.User]().WithOrder("name").List(&users)
>   database.Database[*model.User]().WithOrder("name desc").List(&users)
>   database.Database[*model.User]().WithOrder("name desc, created_at").List(&users)
>   database.Database[*model.User]().WithOrder("name desc, created_at asc").List(&users)
>   ```



#### `_nocache=true`

>`Request`
>
>```bash
>curl --silent --location --request GET 'http://localhost:8080/api/user?_nocache=false' \
>--header 'Authorization: Bearer -'
>```
>
>`Database equivalent`
>
>```go
>database.Database[*model.User]().WithCache(false).List(&users)
>// Or
>database.Database[*model.User]().List(&users)
>```



#### `_column_name=xxx`,`_start_time=xxx`, `_end_time=xxx`

>`Request`
>
>```bash
>curl --silent --location --request GET 'http://localhost:8080/api/user?_column_name=created_at&_start_time=2024-01-01+23%3A59%3A59&_end_time=2030-01-01+23%3A59%3A59' \
>--header 'Authorization: Bearer -'
>```
>
>`Database equivalent`
>
>```go
>database.Database[*model.User]().WithTimeRange("created_at", begin, now).List(&users)
>```

#### `_or=true`

> `Request`
>
> ```bash
> # name=user01&email=user02@gmail.com
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&email=user02%40gmail.com' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
>   "code": 0,
>   "data": {
>   "items": [],
>   "total": 0
>   },
>   "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> // By default, query conditions are combined using AND logic
> database.Database[*model.User]().WithQuery(&model.User{
>   Name:  util.ValueOf("user01"),
>   Email: util.ValueOf("user02@gmail.com"),
> })
> ```
>
> 

> `Request`
>
> ```bash
> # name=user01&email=user02@gmail.com&_or=true
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&email=user02%40gmail.com&_or=true' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "items": [
>    {
>      "id": "user01",
>      "created_at": "2024-12-25T17:22:28.558+08:00",
>      "updated_at": "2024-12-25T17:22:28.558+08:00",
>      "name": "user01",
>      "email": "user01@gmail.com"
>    },
>    {
>      "id": "user02",
>      "created_at": "2024-12-25T17:22:31.581+08:00",
>      "updated_at": "2024-12-25T17:22:31.582+08:00",
>      "name": "user02",
>      "email": "user02@gmail.com"
>    }
>  ],
>  "total": 2
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> // Use OR mode to combine query conditions
> database.Database[*model.User]().WithQuery(&model.User{
>   Name:  util.ValueOf("user01"),
>   Email: util.ValueOf("user02@gmail.com"),
> }, types.QueryConfig{
>   UseOr: true,
> })
> ```

#### `_index=xxx`

>   `Request`
>
>   ```bash
>   curl --silent --location --request GET 'http://localhost:8080/api/user?_index=idx_composite_name_email_createdat' \
>   --header 'Authorization: Bearer -'
>   ```
>
>   `Database equivalent`

  ```go
  // Default behavior - defaults to USE INDEX
  database.Database[*model.User]().WithIndex("idx_composite_name_email_createdby").List(&users)
  
  // Explicit USE INDEX
  database.Database[*model.User]().WithIndex("idx_composite_name_email_createdby", consts.IndexHintUse).List(&users)
  
  // FORCE INDEX for critical performance
  database.Database[*model.User]().WithIndex("idx_composite_name_email_createdby", consts.IndexHintForce).List(&users)
  
  // IGNORE INDEX to avoid using specific index
  database.Database[*model.User]().WithIndex("idx_composite_name_email_createdby", consts.IndexHintIgnore).List(&users)
  ```
>
>   `SQL equivalent`
>
>   ```sql
>   SELECT * FROM `gst_users` USE INDEX (`idx_composite_name_email_createdat`) WHERE `gst_users`.`deleted_at` IS NULL LIMIT 1000
>   ```

#### `_select=xxx`

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_select=name' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "items": [
>    {
>      "id": "user01",
>      "created_at": "2024-12-25T17:22:28.558+08:00",
>      "updated_at": "2024-12-25T17:22:28.558+08:00",
>      "name": "user01"
>    }
>  ],
>  "total": 1
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().WithSelect("name").List(&users)
> ```

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_select=name%2Cemail' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "items": [
>    {
>      "id": "user01",
>      "created_at": "2024-12-25T17:22:28.558+08:00",
>      "updated_at": "2024-12-25T17:22:28.558+08:00",
>      "name": "user01",
>      "email": "user01@gmail.com"
>    }
>  ],
>  "total": 1
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().WithSelect("name", "email").List(&users)
> ```

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01' \
> --header 'Authorization: Bearer -'
> ```
>
> `Response`
>
> ```json
> {
> "code": 0,
> "data": {
>  "items": [
>    {
>      "id": "user01",
>      "created_at": "2024-12-25T17:22:28.558+08:00",
>      "updated_at": "2024-12-25T17:54:42.918+08:00",
>      "name": "user01",
>      "email": "user01_modifed@gmail.com",
>      "avatar": "https://myavataor.com",
>      "sunname": "mysunname",
>      "nickname": "mynickname"
>    }
>  ],
>  "total": 1
> },
> "msg": "success"
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User]().List(&users)
> ```

## Get

> `request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user/user01' \
> --header 'Authorization: Bearer -'
> ```
>
> `response`
>
> ```json
> {
>     "code": 0,
>     "data": {
>        "id": "user01",
>        "created_at": "2024-12-25T11:21:21.134+08:00",
>        "updated_at": "2024-12-25T11:21:21.135+08:00",
>        "name": "user01",
>        "email": "user01@gmail.com"
>     },
>     "msg": "success"
> }
> ```

### query parameters

The useage of`_expand`, `_depth`, `_index`, `_select`, `_nocache` is the same as List query parameters.
