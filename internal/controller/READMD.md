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

The resource id comes from the route parameter only. Batch deletion should use
the `DeleteMany` action instead.

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

## Update

> `Request`
>
> ```bash
> curl --silent --location --request PUT 'http://localhost:8080/api/user/user01' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
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

The resource id comes from the route parameter only. The id carried by the
http body is ignored.

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

#### `_page=number`, `_size=number` for pagination.

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user?_page=1&_size=10' \
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

#### `field_name=value`

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

Bare keys are exact-match filters. For substring (fuzzy) matching on a single
field, use the field operator filter syntax instead: `?name[like]=user01`
(see the `field[op]=value` section below).

#### `_sort_by=xxx`

>   `Request`
>
>   ```bash
>   # _sort_by=name
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name' \
>   --header 'Authorization: Bearer -'
>   
>   # _sort_by=name desc
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name%20desc' \
>   --header 'Authorization: Bearer -'
>   
>   # _sort_by=name desc, created_at
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name%20desc%2C%20created_at' \
>   --header 'Authorization: Bearer -'
>   
>   # _sort_by=name desc, created_at asc
>   curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name%20desc%2C%20created_at%20asc' \
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



#### `_no_cache=true`

>`Request`
>
>```bash
>curl --silent --location --request GET 'http://localhost:8080/api/user?_no_cache=false' \
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



#### `field[op]=value` (field operator filters)

Field-level operator filters (see `parseFiltersQuery` in `query.go`) require
`model.Query` and are always AND-combined with the other conditions. Supported
operators: `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`/`notin` (comma-separated
values), `like`/`notlike` (substring match), `startswith`/`endswith` (anchored
match; a prefix can use an index), and `isnull` (boolean value; works on any
nullable column). Values of the LIKE-based operators are literals, not pattern
language: `%`, `_`, and the escape character are escaped. The bare key stays the
exact business filter, so `?age=10&age[gt]=20` applies both conditions. Unknown
fields or operators, and combining with `_or=true`, return 400; empty values
mean "not filtering".

Service code builds the same filters with the `types.FilterXxx` constructors
(e.g. `types.FilterIn("id", ids)` binds the slice as a whole), whose signatures
lock the value shape each operator expects.

Values are validated against the field's Go type and rejected with 400 when
malformed. Numeric fields require numeric values. Time fields accept the
comparison operators only, with the formats parsed by `parseQueryTime`:
`2006-01-02 15:04:05`, `2006-01-02T15:04[:05]`, `2006-01-02`, RFC 3339 with
explicit offset, and unix seconds/milliseconds; zone-less layouts use the
server's local zone, and a date-only value extends to the end of the day as an
`lte` or `gt` bound. Time ranges combine `gte` and `lte` on the same field;
the framework-managed `created_at`/`updated_at` columns are reachable through
operator filters only.

>`Request`
>
>```bash
># created_at range for July 2024 plus a numeric lower bound
>curl --silent --location --request GET 'http://localhost:8080/api/user?age%5Bgte%5D=18&created_at%5Bgte%5D=2024-07-01&created_at%5Blte%5D=2024-07-31' \
>--header 'Authorization: Bearer -'
>```
>
>`Database equivalent`
>
>```go
>database.Database[*model.User]().WithQuery(nil, types.QueryOptions{
>	AllowEmpty: true,
>	Filters: []types.Filter{
>		types.FilterGte("age", "18"),
>		types.FilterGte("created_at", "2024-07-01 00:00:00"),
>		types.FilterLte("created_at", "2024-07-31 23:59:59.999999999"),
>	},
>}).List(&users)
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
> }, types.QueryOptions{
>   Or: true,
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

The useage of`_expand`, `_depth`, `_index`, `_select`, `_no_cache` is the same as List query parameters.
