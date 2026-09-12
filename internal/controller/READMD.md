## Create

The id is optional: one left out of the body is generated.

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
>     "created_at": "2024-12-25T04:43:21.241Z",
>     "updated_at": "2024-12-25T04:43:21.241Z",
>     "name": "user01",
>     "email": "user01@gmail.com"
>   },
>   "msg": "success",
>   "trace_id": "..."
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
> database.Database[*model.User](ctx).Create(user)
> ```

## Delete

The resource id comes from the route parameter only. Batch deletion should use
the `DeleteMany` action instead. Whether the row is removed or soft deleted
follows the model's `Purge` setting.

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
>     "data": null,
>     "msg": "success",
>     "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> user.SetID("user01")
> database.Database[*model.User](ctx).Delete(user)
> ```

## Update

PUT replaces the whole record: a field the body leaves out is written as its
zero value. The resource id comes from the route parameter only, and a missing
record answers 404.

> `Request`
>
> ```bash
> curl --silent --location --request PUT 'http://localhost:8080/api/user/user01' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
>    "name": "user01_modified",
>    "email": "user01_modified@gmail.com"
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
>        "created_at": "2024-12-25T05:01:01.634Z",
>        "updated_at": "2024-12-25T05:26:18.307Z",
>        "name": "user01_modified",
>        "email": "user01_modified@gmail.com"
>     },
>     "msg": "success",
>     "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> user.ID = "user01"
> user.Name = "user01_modified"
> user.Email = "user01_modified@gmail.com"
> database.Database[*model.User](ctx).Update(user)
> ```

## UpdatePartial

The resource id comes from the route parameter only. The id carried by the
http body is ignored. The handler loads the record, copies the fields present
in the body onto it and saves the whole record, so concurrent patches of one
record resolve as last writer wins unless the model declares `model.Version`.

> `Request`
>
> ```bash
> curl --silent --location --request PATCH 'http://localhost:8080/api/user/user01' \
> --header 'Content-Type: application/json' \
> --header 'Authorization: Bearer -' \
> --data-raw '{
>    "name": "user01_modified",
>    "email": "user01_modified@gmail.com"
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
>        "created_at": "2024-12-25T09:22:28.558Z",
>        "updated_at": "2024-12-26T02:29:32.837Z",
>        "name": "user01_modified",
>        "email": "user01_modified@gmail.com",
>        "avatar": "https://myavatar.com",
>        "surname": "mysurname",
>        "nickname": "mynickname"
>     },
>     "msg": "success",
>     "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> database.Database[*model.User](ctx).Get(user, "user01")
> user.Name = "user01_modified"
> user.Email = "user01_modified@gmail.com"
> database.Database[*model.User](ctx).Update(user)
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
>         "created_at": "2024-12-25T03:21:21.134Z",
>         "updated_at": "2024-12-25T03:21:21.134Z",
>         "name": "user01",
>         "email": "user01@gmail.com"
>       },
>       {
>         "id": "user02",
>         "created_at": "2024-12-25T03:23:18.017Z",
>         "updated_at": "2024-12-25T03:23:18.017Z",
>         "name": "user02",
>         "email": "user02@gmail.com"
>       }
>     ],
>     "total": 2
>   },
>   "msg": "success",
>   "trace_id": "..."
> }
> ```

### Query Parameters

#### `_page=number`, `_size=number` for pagination.

Paging requires the model to embed `model.Pagination`, which `model.Query`
includes.

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
> database.Database[*model.User](ctx).WithPagination(1, 10).List(&users)
> ```

#### `_expand=all`

`_expand` preloads the associations the model lists in `Expands()`. Names match
ignoring case and snake case punctuation, several names are comma-separated,
and `all` selects every association. List accepts it when the model embeds
`model.Query`; Get accepts it as well, and the examples below read one record.

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
>  "created_at": "2024-12-25T07:36:25.156Z",
>  "updated_at": "2024-12-25T07:36:25.156Z",
>  "name": "fruit",
>  "status": 1,
>  "parent_id": "root",
>  "children": [
>    {
>      "id": "apple",
>      "created_at": "2024-12-25T07:36:25.156Z",
>      "updated_at": "2024-12-25T07:36:25.156Z",
>      "name": "apple",
>      "status": 1,
>      "parent_id": "fruit"
>    },
>    {
>      "id": "banana",
>      "created_at": "2024-12-25T07:36:25.156Z",
>      "updated_at": "2024-12-25T07:36:25.156Z",
>      "name": "banana",
>      "status": 1,
>      "parent_id": "fruit"
>    }
>  ],
>  "parent": {
>    "id": "root",
>    "created_at": "2024-12-25T07:36:25.156Z",
>    "updated_at": "2024-12-25T07:36:25.156Z",
>    "name": "root",
>    "status": 0,
>    "parent_id": "root"
>  }
> },
> "msg": "success",
> "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> category := new(model.Category)
> database.Database[*model.Category](ctx).WithExpand(category.Expands()).Get(category, "fruit")
> ```

#### `_expand=children`

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
>  "created_at": "2024-12-25T07:36:25.156Z",
>  "updated_at": "2024-12-25T07:36:25.156Z",
>  "name": "fruit",
>  "status": 1,
>  "parent_id": "root",
>  "children": [
>    {
>      "id": "apple",
>      "created_at": "2024-12-25T07:36:25.156Z",
>      "updated_at": "2024-12-25T07:36:25.156Z",
>      "name": "apple",
>      "status": 1,
>      "parent_id": "fruit"
>    },
>    {
>      "id": "banana",
>      "created_at": "2024-12-25T07:36:25.156Z",
>      "updated_at": "2024-12-25T07:36:25.156Z",
>      "name": "banana",
>      "status": 1,
>      "parent_id": "fruit"
>    }
>  ]
> },
> "msg": "success",
> "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> category := new(model.Category)
> database.Database[*model.Category](ctx).WithExpand([]string{"Children"}).Get(category, "fruit")
> ```

#### `_expand=children`,`_depth=3`

`_depth` expands a slice association such as `children` that many levels deep,
from 1 to 10; a missing or out-of-range value means 1, and a non-slice
association such as `parent` ignores it.

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
>  "created_at": "2024-12-25T07:36:25.156Z",
>  "updated_at": "2024-12-25T07:36:25.156Z",
>  "name": "fruit",
>  "status": 1,
>  "parent_id": "root",
>  "children": [
>    {
>      "id": "apple",
>      "created_at": "2024-12-25T07:36:25.156Z",
>      "updated_at": "2024-12-25T07:36:25.156Z",
>      "name": "apple",
>      "status": 1,
>      "parent_id": "fruit",
>      "children": [
>        {
>          "id": "apple1",
>          "created_at": "2024-12-25T07:36:25.156Z",
>          "updated_at": "2024-12-25T07:36:25.156Z",
>          "name": "apple1",
>          "status": 1,
>          "parent_id": "apple"
>        },
>        {
>          "id": "apple2",
>          "created_at": "2024-12-25T07:36:25.156Z",
>          "updated_at": "2024-12-25T07:36:25.156Z",
>          "name": "apple2",
>          "status": 1,
>          "parent_id": "apple"
>        }
>      ]
>    },
>    {
>      "id": "banana",
>      "created_at": "2024-12-25T07:36:25.156Z",
>      "updated_at": "2024-12-25T07:36:25.156Z",
>      "name": "banana",
>      "status": 1,
>      "parent_id": "fruit",
>      "children": [
>        {
>          "id": "banana1",
>          "created_at": "2024-12-25T07:36:25.156Z",
>          "updated_at": "2024-12-25T07:36:25.156Z",
>          "name": "banana1",
>          "status": 1,
>          "parent_id": "banana"
>        },
>        {
>          "id": "banana2",
>          "created_at": "2024-12-25T07:36:25.156Z",
>          "updated_at": "2024-12-25T07:36:25.156Z",
>          "name": "banana2",
>          "status": 1,
>          "parent_id": "banana"
>        }
>      ]
>    }
>  ]
> },
> "msg": "success",
> "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> category := new(model.Category)
> database.Database[*model.Category](ctx).WithExpand([]string{"Children.Children.Children"}).Get(category, "fruit")
> ```

#### `field_name=value`

Bare keys are exact-match filters on the model's fields. A field's URL name is
its `query` tag, falling back to its `json` tag name and then to a name derived
from the field name; `query:"-"` keeps a field out of URL filtering. For
example:

```golang
type User struct {
	model.Base

	Name     string `json:"name,omitempty" query:"name"`
	Email    string `json:"email,omitempty" query:"email"`
	Avatar   string `json:"avatar,omitempty" query:"avatar"`
	Surname  string `json:"surname,omitempty" query:"surname"`
	Nickname string `json:"nickname,omitempty" query:"nickname"`
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
> ```json
> {
>  "code": 0,
>  "data": {
>      "items": [
>          {
>              "id": "user01",
>              "created_at": "2024-12-25T09:22:28.558Z",
>              "updated_at": "2024-12-25T09:22:28.558Z",
>              "name": "user01",
>              "email": "user01@gmail.com"
>          }
>      ],
>      "total": 1
>  },
>  "msg": "success",
>  "trace_id": "..."
> }
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User](ctx).WithQuery(&model.User{Name: "user01"}).List(&users)
> ```

For substring (fuzzy) matching on a single field, use the field operator filter
syntax instead: `?name[like]=user01` (see the `field[op]=value` section below).

#### `_sort_by=xxx`

> `Request`
>
> ```bash
> # _sort_by=name
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name' \
> --header 'Authorization: Bearer -'
>
> # _sort_by=name desc
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name%20desc' \
> --header 'Authorization: Bearer -'
>
> # _sort_by=name desc, created_at
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name%20desc%2C%20created_at' \
> --header 'Authorization: Bearer -'
>
> # _sort_by=name desc, created_at asc
> curl --silent --location --request GET 'http://localhost:8080/api/user?name=user01&_sort_by=name%20desc%2C%20created_at%20asc' \
> --header 'Authorization: Bearer -'
> ```
>
> `Database equivalent`
>
> ```go
> database.Database[*model.User](ctx).WithQuery(&model.User{Name: "user01"}).WithOrder(types.Asc("name")).List(&users)
> database.Database[*model.User](ctx).WithQuery(&model.User{Name: "user01"}).WithOrder(types.Desc("name")).List(&users)
> database.Database[*model.User](ctx).WithQuery(&model.User{Name: "user01"}).WithOrder(types.Desc("name"), types.Asc("created_at")).List(&users)
> ```

#### `field[op]=value` (field operator filters)

Field-level operator filters (see `internal/urlquery.Filters`) require
`model.Query` and are always AND-combined with the other conditions. Supported
operators: `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`/`notin` (comma-separated
values), `like`/`notlike` (substring match), `startswith`/`endswith` (anchored
match; a prefix can use an index), and `isnull` (boolean value; works on any
nullable column). Values of the LIKE-based operators are literals, not pattern
language: `%`, `_`, and the escape character are escaped. The bare key stays the
exact business filter, so `?age=10&age[gt]=20` applies both conditions. Unknown
fields or operators return 400; empty values
mean "not filtering".

Service code builds the same filters through the typed column references
`gg gen` generates next to each model, for example
`sample.RecordCols.Status.In(sample.RecordStatusPending)`: the column name and the
value type are both checked by the compiler, so a renamed column or a wrong
value type fails the build instead of the query. Generic code, which has no
concrete model and so no generated references, mints one for its type
parameter, for example `types.NewColumn[M, string]("id").In(ids...)`, and keeps
the value type checked. The `types.FilterXxx` constructors with a string column
name are left to code that learns the column only at run time, such as the
framework's own URL parsing.

A service that implements a list action itself reaches the same parsing
through the `QueryXxx` methods on its `service.Base`. Service code also has
`types.FilterOr` and `types.FilterAnd` to group filters, which is the only way
a query expresses OR. Groups are deliberately
not part of the URL contract: a client cannot change how conditions combine,
so a mandatory service-side filter can never be OR-ed away.

Values are validated against the field's Go type and rejected with 400 when
malformed. Numeric fields require numeric values. Time fields accept the
comparison operators only, and `parseQueryTime` accepts RFC 3339 alone, with
an explicit offset and fractional seconds up to nanoseconds: zone-less
spellings, date-only values and unix timestamps return 400, because a
zone-less value names a different instant in every server zone. A `+` offset
must be percent-encoded as `%2B` in the query string (or written as `Z` or a
negative offset). The bound reaches the database as the UTC wall clock in
`types.FilterTimeLayout`. Time ranges combine `gte` and `lte` on the same
field; the framework-managed `created_at`/`updated_at` columns also take their
bare key as an exact-match filter.

>`Request`
>
>```bash
># created_at range for July 2024 plus a numeric lower bound
>curl --silent --location --request GET 'http://localhost:8080/api/user?age%5Bgte%5D=18&created_at%5Bgte%5D=2024-07-01T00:00:00Z&created_at%5Blte%5D=2024-07-31T23:59:59Z' \
>--header 'Authorization: Bearer -'
>```
>
>`Database equivalent`
>
>```go
>database.Database[*model.User](ctx).WithQuery(nil, types.QueryOptions{
>	AllowEmpty: true,
>	Filters: []types.Filter{
>		types.FilterGte("age", "18"),
>		types.FilterGte("created_at", "2024-07-01 00:00:00"),
>		types.FilterLte("created_at", "2024-07-31 23:59:59"),
>	},
>}).List(&users)
>```

## Get

`GET /api/user/:id` reads one record by the route id. It takes `_expand` and
`_depth` the same way List does; see the examples above.

> `Request`
>
> ```bash
> curl --silent --location --request GET 'http://localhost:8080/api/user/user01' \
> --header 'Authorization: Bearer -'
> ```
>
> `Database equivalent`
>
> ```go
> user := new(model.User)
> database.Database[*model.User](ctx).Get(user, "user01")
> ```
