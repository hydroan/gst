package database

import (
	"context"
	"reflect"
	"slices"
	"time"

	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

// dryRunReadSession builds read SQL through GORM without executing database I/O.
func (db *database[M]) dryRunReadSession() *gorm.DB {
	return db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)})
}

// List retrieves multiple records from the database based on applied conditions.
// Returns all records if no conditions are specified, or filtered records with WithQuery.
// Supports caching, pagination, sorting, and eager loading of associations.
//
// Parameters:
//   - dest: Pointer to the result slice. The pointer itself must not be nil.
//     The slice value may be nil or initialized with make; List replaces its
//     contents with the query result.
//
// Features:
//   - Automatic result caching when enabled
//   - Supports pagination with WithLimit/WithOffset
//   - Supports sorting with WithOrder
//   - Supports filtering with WithQuery
//   - Supports eager loading with WithExpand
//
// Example:
//
//	var users []*User
//	List(&users)  // Get all users
//
//	users := make([]*User, 0)
//	WithQuery(&User{Status: "active"}).List(&users)  // Get active users
//	WithLimit(10).WithOffset(20).List(&users)  // Paginated results
func (db *database[M]) List(dest *[]M) (err error) {
	defer db.reset()

	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, span := db.trace("List")
	defer done(err)
	if dest == nil {
		return ErrNilDest
	}

	begin := time.Now()
	var key string
	// set selected columns.
	if len(db.selectColumns) > 0 {
		db.ins = db.ins.Select(db.selectColumns)
	}
	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		db.applyCursorPagination()
		tx := db.dryRunReadSession().Table(tableName).Find(dest)
		return db.collectSQL(tx)
	}
	if !db.enableCache {
		goto QUERY
	}
	_, _, key = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).Find(dest).Statement, "list")
	if _dest, e := cache.Cache[[]M]().WithContext(ctx).Get(key); e != nil {
		// metrics.CacheMiss.WithLabelValues("list", db.typ.Name()).Inc()
		goto QUERY
	} else {
		// metrics.CacheHit.WithLabelValues("list", db.typ.Name()).Inc()
		*dest = _dest
		logger.Cache.Infow("list from cache", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		return nil
	}

	// =============================
	// ===== BEGIN redis cache =====
	// =============================
	// begin := time.Now()
	// var key string
	// var shouldDecode bool // if cache is nil or cache[0] is nil, we should decod the queryed cache in to "dest".
	// var _cache []byte
	// if !db.enableCache {
	// 	goto QUERY
	// }
	// if !config.App.RedisConfig.Enable {
	// 	goto QUERY
	// }
	// _, key = buildCacheKey(db.db.Session(&gorm.Session{DryRun: true}).Find(dest).Statement, "list")
	// if len(cache) == 0 {
	// 	shouldDecode = true
	// } else {
	// 	if cache[0] == nil {
	// 		shouldDecode = true
	// 	}
	// }
	// if shouldDecode {
	// 	var _dest []M
	// 	if _dest, err = redis.GetML[M](db.ctx, key); err == nil {
	// 		val := reflect.ValueOf(dest)
	// 		if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Slice {
	// 			return ErrNotPtrSlice
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableSlice
	// 		}
	// 		if !val.Elem().CanSet() {
	// 			return ErrNotSetSlice
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_dest))
	// 		logger.Cache.Infow("list and decode from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// } else {
	// 	if _cache, err = redis.Get(db.ctx, key); err == nil {
	// 		val := reflect.ValueOf(cache[0])
	// 		if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Slice {
	// 			return ErrNotPtrSlice
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableSlice
	// 		}
	// 		if !val.Elem().CanSet() {
	// 			return ErrNotSetSlice
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_cache))
	// 		logger.Cache.Infow("list from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// }
	// if !errors.Is(err, redis.ErrKeyNotExists) {
	// 	logger.Cache.Error(err)
	// 	return err
	// }
	// // Not Found cache and continue.
	// ===========================
	// ===== END redis cache =====
	// ===========================

QUERY:
	var empty M // call nil value M will cause panic.
	// Invoke model hook: ListBefore.
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_LIST_BEFORE, span, func(spanCtx context.Context) error {
			for i := range *dest {
				if !reflect.DeepEqual(empty, (*dest)[i]) {
					if err = (*dest)[i].ListBefore(types.NewModelContext(spanCtx)); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	// if err = db.db.Find(dest).Error; err != nil {
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	// apply cursor-based pagination.
	db.applyCursorPagination()
	if err = db.ins.Table(tableName).Find(dest).Error; err != nil {
		return err
	}
	// If cursor-based pagination is enabled and this is a previous page query,
	// reverse the list to mantain the original sort order.
	if db.enableCursor && !db.cursorNext {
		slices.Reverse(*dest)
	}

	// Invoke model hook: ListAfter()
	if !db.noHook {
		if err = traceModelHook[M](db.ctx, consts.PHASE_LIST_AFTER, span, func(spanCtx context.Context) error {
			for i := range *dest {
				if !reflect.DeepEqual(empty, (*dest)[i]) {
					if err = (*dest)[i].ListAfter(types.NewModelContext(spanCtx)); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	// Cache the result.
	// if db.enableCache && config.App.RedisConfig.Enable {
	// 	logger.Cache.Infow("list from database", "cost", time.Since(begin).String(), "key", key)
	// 	go func() {
	// 		if err = redis.SetML[M](key, *dest); err != nil {
	// 			logger.Cache.Error(err)
	// 		}
	// 	}()
	// }
	if db.enableCache {
		logger.Cache.Infow("list from database", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		_ = cache.Cache[[]M]().WithContext(ctx).Set(key, *dest, config.App.Cache.Expiration)
	}

	return nil
}

// // Find equal to WithQuery(condition).List()
// // More detail see `List` document.
// func (db *database[T]) Find(dest *[]T, query T) error {
// 	return db.db.Where(query).Find(dest).Error
// }

// Get retrieves a single record from the database by its primary key (ID).
// Supports automatic caching to improve performance for frequently accessed records.
// Executes GetBefore and GetAfter model hooks unless disabled with WithoutHook.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//   - id: Primary key value of the record to retrieve
//
// Returns ErrIDRequired if id is empty. A missing record leaves dest empty and does not return
// gorm.ErrRecordNotFound because Get uses GORM Find.
//
// Features:
//   - Automatic result caching when enabled
//   - Cache-first lookup for improved performance
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//
// Example:
//
//	var user User
//	Get(&user, "user123")  // Get user by ID
//	WithExpand([]string{"Orders"}).Get(&user, "user123")  // Get user with orders
func (db *database[M]) Get(dest M, id string) (err error) {
	defer db.reset()

	val := reflect.ValueOf(dest)
	if !val.IsValid() || val.IsNil() {
		return ErrNilDest
	}
	if len(id) == 0 {
		return ErrIDRequired
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, span := db.trace("Get")
	defer done(err)

	begin := time.Now()
	var key string
	// set selected columns.
	if len(db.selectColumns) > 0 {
		db.ins = db.ins.Select(db.selectColumns)
	}
	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		dryRunDest := cloneDryRunModel(dest)
		dryRunDest.ClearID()
		if len(tableName) == 0 {
			tx := db.dryRunReadSession().Where("id = ?", id).Find(dryRunDest)
			return db.collectSQL(tx)
		}
		tx := db.dryRunReadSession().Table(tableName).Where(db.quoteTableColumn(tableName, "id")+" = ?", id).Find(dryRunDest)
		return db.collectSQL(tx)
	}
	if !db.enableCache {
		goto QUERY
	}
	_, _, key = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).Where("id = ?", id).Find(dest).Statement, "get", id)
	if _dest, e := cache.Cache[M]().WithContext(ctx).Get(key); e != nil {
		// metrics.CacheMiss.WithLabelValues("get", db.typ.Name()).Inc()
		goto QUERY
	} else {
		// metrics.CacheHit.WithLabelValues("get", db.typ.Name()).Inc()
		val := reflect.ValueOf(dest)
		if val.Kind() != reflect.Pointer {
			return ErrNotPtrStruct
		}
		if !val.Elem().CanAddr() {
			return ErrNotAddressableModel
		}
		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
		logger.Cache.Infow("get from cache", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		return nil
	}

	// =============================
	// ===== BEGIN redis cache =====
	// =============================
	// begin := time.Now()
	// var key string
	// var shouldDecode bool // if cache is nil or cache[0] is nil, we should decod the queryed cache in to "dest".
	// if !db.enableCache {
	// 	goto QUERY
	// }
	// if !config.App.RedisConfig.Enable {
	// 	goto QUERY
	// }
	// // _, key = BuildKey(db.db.Session(&gorm.Session{DryRun: true}).Where("id = ?", id).Find(dest).Statement, "get")
	// // 我发现这个 id 的值怎么都无法填充到 sql 语句中, 所以直接用 id 作为 key 的一部分,而不是用 sql 语句
	// // 如果不用 id 作为 redis key, 那么多个 get 的语句相同则 key 相同,获取到的都是同一个缓存.
	// _, key = buildCacheKey(db.db.Session(&gorm.Session{DryRun: true}).Where("id = ?", id).Find(dest).Statement, "get", id)
	// if len(cache) == 0 {
	// 	shouldDecode = true
	// } else {
	// 	if cache[0] == nil {
	// 		shouldDecode = true
	// 	}
	// }
	// if shouldDecode { // query cache from redis and decoded into 'dest'.
	// 	var _dest M
	// 	if _dest, err = redis.GetM[M](key); err == nil {
	// 		val := reflect.ValueOf(dest)
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrStruct
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableModel
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
	// 		logger.Cache.Infow("get and decode from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// } else {
	// 	var _cache []byte
	// 	if _cache, err = redis.Get(db.ctx, key); err == nil {
	// 		val := reflect.ValueOf(cache[0])
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrSlice
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableSlice
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_cache))
	// 		logger.Cache.Infow("get from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// }
	// if err != redis.ErrKeyNotExists {
	// 	logger.Cache.Error(err)
	// 	return err
	// }
	// // Not Found cache, continue query database.
	// ===========================
	// ===== END redis cache =====
	// ===========================

QUERY:
	var empty M // call nil value M will cause panic.
	// Invoke model hook: GetBefore.
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// if err = db.db.Where("id = ?", id).Find(dest).Error; err != nil {
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	// Use an explicit WHERE clause instead of relying on primary key fields
	// already present on dest.
	//
	// dest.SetID(id)
	// if err = db.db.Table(tableName).Find(dest).Error; err != nil {
	// 	return err
	// }
	if len(tableName) == 0 {
		_, tableName, _ = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).Where("id = ?", id).Find(dest).Statement, "get", id)
	}
	dest.ClearID()
	if err = db.ins.Table(tableName).Where(db.quoteTableColumn(tableName, "id")+" = ?", id).Find(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter.
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// // Cache the result.
	// if db.enableCache && config.App.RedisConfig.Enable {
	// 	logger.Cache.Infow("get from database", "cost", time.Since(begin).String(), "key", key)
	// 	go func() {
	// 		if err = redis.SetM[M](key, dest); err != nil {
	// 			logger.Cache.Error(err)
	// 		}
	// 	}()
	// }
	if db.enableCache {
		logger.Cache.Infow("get from database", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		_ = cache.Cache[M]().WithContext(ctx).Set(key, dest, config.App.Cache.Expiration)
	}
	return nil
}

// Count returns the total number of records matching the current query conditions.
// Supports automatic caching to improve performance for frequently executed count queries.
// Applies all previously set query conditions (WHERE, JOIN, etc.) to the count operation.
//
// Parameters:
//   - count: Pointer to int64 where the result count will be stored
//
// Returns database errors if the query fails.
//
// Features:
//   - Automatic result caching when enabled
//   - Cache-first lookup for improved performance
//   - Respects query modifiers such as WHERE and JOIN
//   - Uses LIMIT(-1) to clear existing LIMIT clauses and count all matching rows
//
// Example:
//
//	var total int64
//	WithQuery(&User{Status: "active"}).Count(&total)  // Count active records
//	WithQuery(&User{Name: "john"}).Count(&total)      // Count records matching name
//
// Note: The count parameter must be a non-nil pointer to int64.
func (db *database[M]) Count(count *int64) (err error) {
	defer db.reset()

	if count == nil {
		return ErrNilCount
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, _ := db.trace("Count")
	defer done(err)

	begin := time.Now()
	var key string
	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		tx := db.dryRunReadSession().Table(tableName).Model(*new(M)).Limit(-1).Count(count)
		return db.collectSQL(tx)
	}
	if !db.enableCache {
		goto QUERY
	}
	_, _, key = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).Model(*new(M)).Count(count).Statement, "count")
	if _cache, e := cache.Cache[int64]().WithContext(ctx).Get(key); e != nil {
		// metrics.CacheMiss.WithLabelValues("count", db.typ.Name()).Inc()
		goto QUERY
	} else {
		// metrics.CacheHit.WithLabelValues("count", db.typ.Name()).Inc()
		*count = _cache
		logger.Cache.Infow("count from cache", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		return err
	}

	// =============================
	// ===== BEGIN redis cache =====
	// =============================
	// begin := time.Now()
	// var key string
	// var _cache int64
	// if count == nil {
	// 	return nil
	// }
	// if !db.enableCache {
	// 	goto QUERY
	// }
	// if !config.App.RedisConfig.Enable {
	// 	goto QUERY
	// }
	// fmt.Println("---- buildCacheKey count")
	// _, key = buildCacheKey(db.db.Session(&gorm.Session{DryRun: true}).Model(*new(M)).Count(count).Statement, "count")
	// if _cache, err = redis.GetInt(db.ctx, key); err == nil {
	// 	*count = _cache
	// 	logger.Cache.Infow("count from cache", "cost", time.Since(begin).String(), "key", key)
	// 	return
	// }
	// if !errors.Is(err, redis.ErrKeyNotExists) {
	// 	logger.Cache.Error(err)
	// 	return err
	// }
	// // NOT FOUND cache, continue query.
	// ===========================
	// ===== END redis cache =====
	// ===========================

QUERY:
	// if err = db.db.Model(*new(M)).Count(count).Error; err != nil {
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	if err = db.ins.Table(tableName).Model(*new(M)).Limit(-1).Count(count).Error; err != nil {
		logger.Cache.Error(err)
		return err
	}
	// if db.enableCache && config.App.RedisConfig.Enable {
	// 	logger.Cache.Infow("count from database", "cost", time.Since(begin).String(), "key", key)
	// 	go func() {
	// 		if err = redis.Set(db.ctx, key, *count); err != nil {
	// 			logger.Cache.Error(err)
	// 		}
	// 	}()
	//
	// }
	if db.enableCache {
		logger.Cache.Infow("count from database", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		_ = cache.Cache[int64]().WithContext(ctx).Set(key, *count, config.App.Cache.Expiration)

	}
	return nil
}

// First retrieves the first record from the database ordered by primary key.
// Supports automatic caching to improve performance for frequently accessed queries.
// Applies all previously set query conditions and returns the first matching record.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//
// Returns database errors if no record is found or query fails.
//
// Features:
//   - Automatic result caching when enabled
//   - Cache-first lookup for improved performance
//   - Supports all query modifiers (WHERE, ORDER BY, etc.)
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//
// Example:
//
//	var user User
//	First(&user)  // Get first user by primary key
//	WithQuery(&User{Status: "active"}).First(&user)  // Get first active user
//	WithOrder("created_at DESC").First(&user)  // Get newest user
func (db *database[M]) First(dest M) (err error) {
	defer db.reset()

	val := reflect.ValueOf(dest)
	if !val.IsValid() || val.IsNil() {
		return ErrNilDest
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, span := db.trace("First")
	defer done(err)

	begin := time.Now()
	var key string
	// set selected columns.
	if len(db.selectColumns) > 0 {
		db.ins = db.ins.Select(db.selectColumns)
	}
	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		tx := db.dryRunReadSession().Table(tableName).First(dest)
		return db.collectSQL(tx)
	}
	if !db.enableCache {
		goto QUERY
	}
	_, _, key = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).First(dest).Statement, "first")
	if _dest, e := cache.Cache[M]().WithContext(ctx).Get(key); e != nil {
		// metrics.CacheMiss.WithLabelValues("first", db.typ.Name()).Inc()
		goto QUERY
	} else {
		// metrics.CacheHit.WithLabelValues("first", db.typ.Name()).Inc()
		val := reflect.ValueOf(dest)
		if val.Kind() != reflect.Pointer {
			return ErrNotPtrStruct
		}
		if !val.Elem().CanAddr() {
			return ErrNotAddressableModel
		}
		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
		logger.Cache.Infow("first from cache", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		return nil // Found cache and return.
	}

	// =============================
	// ===== BEGIN redis cache =====
	// =============================
	// begin := time.Now()
	// var key string
	// var shouldDecode bool // if cache is nil or cache[0] is nil, we should decod the queryed cache in to "dest".
	// if !db.enableCache {
	// 	goto QUERY
	// }
	// if !config.App.RedisConfig.Enable {
	// 	goto QUERY
	// }
	// _, key = buildCacheKey(db.db.Session(&gorm.Session{DryRun: true}).First(dest).Statement, "first")
	// if len(cache) == 0 {
	// 	shouldDecode = true
	// } else {
	// 	if cache[0] == nil {
	// 		shouldDecode = true
	// 	}
	// }
	// if shouldDecode { // query cache from redis and decode into 'dest'.
	// 	var _dest M
	// 	if _dest, err = redis.GetM[M](key); err == nil {
	// 		val := reflect.ValueOf(dest)
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrStruct
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableModel
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
	// 		logger.Cache.Infow("first and decode from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// } else {
	// 	var _cache []byte
	// 	if _cache, err = redis.Get(db.ctx, key); err == nil {
	// 		val := reflect.ValueOf(cache[0])
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrSlice
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableSlice
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_cache))
	// 		logger.Cache.Infow("first from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// 	if err != redis.ErrKeyNotExists {
	// 		logger.Cache.Error(err)
	// 		return err
	// 	}
	// }
	// Not Found cache, continue query database.
	// ===========================
	// ===== END redis cache =====
	// ===========================

QUERY:
	var empty M // call nil value M will cause panic.
	// Invoke model hook: GetBefore
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// if err = db.db.First(dest).Error; err != nil {
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	if err = db.ins.Table(tableName).First(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// // Cache the result.
	// if db.enableCache && config.App.RedisConfig.Enable {
	// 	logger.Cache.Infow("first from database", "cost", time.Since(begin).String(), "key", key)
	// 	go func() {
	// 		if err = redis.SetM[M](key, dest); err != nil {
	// 			logger.Cache.Error(err)
	// 		}
	// 	}()
	// }
	if db.enableCache {
		logger.Cache.Infow("first from database", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		_ = cache.Cache[M]().WithContext(ctx).Set(key, dest, config.App.Cache.Expiration)
	}
	return nil
}

// Last retrieves the last record from the database ordered by primary key.
// Supports automatic caching to improve performance for frequently accessed queries.
// Applies all previously set query conditions and returns the last matching record.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//
// Returns database errors if no record is found or query fails.
//
// Features:
//   - Automatic result caching when enabled
//   - Cache-first lookup for improved performance
//   - Supports all query modifiers (WHERE, ORDER BY, etc.)
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//   - Executes GetBefore and GetAfter model hooks unless disabled
//
// Example:
//
//	var user User
//	Last(&user)  // Get last user by primary key
//	WithQuery(&User{Status: "active"}).Last(&user)  // Get last active user
//	WithOrder("created_at ASC").Last(&user)  // Get oldest user (with custom order)
func (db *database[M]) Last(dest M) (err error) {
	defer db.reset()

	val := reflect.ValueOf(dest)
	if !val.IsValid() || val.IsNil() {
		return ErrNilDest
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, span := db.trace("Last")
	defer done(err)

	begin := time.Now()
	var key string
	// set selected columns.
	if len(db.selectColumns) > 0 {
		db.ins = db.ins.Select(db.selectColumns)
	}
	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		tx := db.dryRunReadSession().Table(tableName).Last(dest)
		return db.collectSQL(tx)
	}
	if !db.enableCache {
		goto QUERY
	}
	_, _, key = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).Last(dest).Statement, "last")
	if _dest, e := cache.Cache[M]().WithContext(ctx).Get(key); e != nil {
		// metrics.CacheMiss.WithLabelValues("last", db.typ.Name()).Inc()
		goto QUERY
	} else {
		// metrics.CacheHit.WithLabelValues("last", db.typ.Name()).Inc()
		val := reflect.ValueOf(dest)
		if val.Kind() != reflect.Pointer {
			return ErrNotPtrStruct
		}
		if !val.Elem().CanAddr() {
			return ErrNotAddressableModel
		}
		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
		logger.Cache.Infow("last from cache", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		return nil // Found cache and return.
	}

	// =============================
	// ===== BEGIN redis cache =====
	// =============================
	// begin := time.Now()
	// var key string
	// var shouldDecode bool // if cache is nil or cache[0] is nil, we should decod the queryed cache in to "dest".
	// if !db.enableCache {
	// 	goto QUERY
	// }
	// if !config.App.RedisConfig.Enable {
	// 	goto QUERY
	// }
	// _, key = buildCacheKey(db.db.Session(&gorm.Session{DryRun: true}).First(dest).Statement, "last")
	// if len(cache) == 0 {
	// 	shouldDecode = true
	// } else {
	// 	if cache[0] == nil {
	// 		shouldDecode = true
	// 	}
	// }
	// if shouldDecode { // query cache from redis and decode into 'dest'.
	// 	var _dest M
	// 	if _dest, err = redis.GetM[M](key); err == nil {
	// 		val := reflect.ValueOf(dest)
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrStruct
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableModel
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
	// 		logger.Cache.Infow("last and decode from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// } else {
	// 	var _cache []byte
	// 	if _cache, err = redis.Get(db.ctx, key); err == nil {
	// 		val := reflect.ValueOf(cache[0])
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrSlice
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableSlice
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_cache))
	// 		logger.Cache.Infow("last from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// }
	// if err != redis.ErrKeyNotExists {
	// 	logger.Cache.Error(err)
	// 	return err
	// }
	// // Not Found cache, continue query database.
	// ===========================
	// ===== END redis cache =====
	// ===========================

QUERY:
	var empty M // call nil value M will cause panic.
	// Invoke model hook: GetBefore.
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// if err = db.db.Last(dest).Error; err != nil {
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	if err = db.ins.Table(tableName).Last(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// // Cache the result.
	// if db.enableCache && config.App.RedisConfig.Enable {
	// 	logger.Cache.Infow("last from database", "cost", time.Since(begin).String(), "key", key)
	// 	go func() {
	// 		if err = redis.SetM[M](key, dest); err != nil {
	// 			logger.Cache.Error(err)
	// 		}
	// 	}()
	// }
	if db.enableCache {
		logger.Cache.Infow("last from database", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		_ = cache.Cache[M]().WithContext(ctx).Set(key, dest, config.App.Cache.Expiration)
	}
	return nil
}

// Take retrieves the first record from the database in no specified order.
// Unlike First/Last which order by primary key, Take returns any matching record.
// Supports automatic caching to improve performance for frequently accessed queries.
//
// Parameters:
//   - dest: Pointer to model instance where the result will be stored
//
// Returns database errors if no record is found or query fails.
//
// Features:
//   - Automatic result caching when enabled
//   - Cache-first lookup for improved performance
//   - Supports all query modifiers (WHERE, JOIN, etc.)
//   - Supports eager loading with WithExpand
//   - Supports field selection with WithSelect
//   - Executes GetBefore and GetAfter model hooks unless disabled
//   - No ordering applied (fastest single record retrieval)
//
// Example:
//
//	var user User
//	Take(&user)  // Get any user record
//	WithQuery(&User{Status: "active"}).Take(&user)  // Get any active user
func (db *database[M]) Take(dest M) (err error) {
	defer db.reset()

	val := reflect.ValueOf(dest)
	if !val.IsValid() || val.IsNil() {
		return ErrNilDest
	}
	if err = db.prepare(); err != nil {
		return err
	}
	done, ctx, span := db.trace("Take")
	defer done(err)

	begin := time.Now()
	var key string
	// set selected columns.
	if len(db.selectColumns) > 0 {
		db.ins = db.ins.Select(db.selectColumns)
	}
	if db.dryRun {
		tableName := db.m.GetTableName()
		if len(db.tableName) > 0 {
			tableName = db.tableName
		}
		tx := db.dryRunReadSession().Table(tableName).Take(dest)
		return db.collectSQL(tx)
	}
	if !db.enableCache {
		goto QUERY
	}
	_, _, key = buildCacheKey(db.ins.Session(&gorm.Session{DryRun: true, Logger: glogger.Default.LogMode(glogger.Silent)}).First(dest).Statement, "take")
	if _dest, e := cache.Cache[M]().WithContext(ctx).Get(key); e != nil {
		// metrics.CacheMiss.WithLabelValues("take", db.typ.Name()).Inc()
		goto QUERY
	} else {
		// metrics.CacheHit.WithLabelValues("take", db.typ.Name()).Inc()
		val := reflect.ValueOf(dest)
		if val.Kind() != reflect.Pointer {
			return ErrNotPtrStruct
		}
		if !val.Elem().CanAddr() {
			return ErrNotAddressableModel
		}
		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
		logger.Cache.Infow("take from cache", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		return nil // Found cache and return.
	}

	// =============================
	// ===== BEGIN redis cache =====
	// =============================
	// begin := time.Now()
	// var key string
	// var shouldDecode bool // if cache is nil or cache[0] is nil, we should decod the queryed cache in to "dest".
	// if !db.enableCache {
	// 	goto QUERY
	// }
	// if !config.App.RedisConfig.Enable {
	// 	goto QUERY
	// }
	// _, key = buildCacheKey(db.db.Session(&gorm.Session{DryRun: true}).First(dest).Statement, "take")
	// if len(cache) == 0 {
	// 	shouldDecode = true
	// } else {
	// 	if cache[0] == nil {
	// 		shouldDecode = true
	// 	}
	// }
	// if shouldDecode { // query cache from redis and decode into 'dest'.
	// 	var _dest M
	// 	if _dest, err = redis.GetM[M](key); err == nil {
	// 		val := reflect.ValueOf(dest)
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrStruct
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableModel
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_dest).Elem()) // the type of M is pointer to struct.
	// 		logger.Cache.Infow("get and decode from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// } else {
	// 	var _cache []byte
	// 	if _cache, err = redis.Get(db.ctx, key); err == nil {
	// 		val := reflect.ValueOf(cache[0])
	// 		if val.Kind() != reflect.Pointer {
	// 			return ErrNotPtrSlice
	// 		}
	// 		if !val.Elem().CanAddr() {
	// 			return ErrNotAddressableSlice
	// 		}
	// 		val.Elem().Set(reflect.ValueOf(_cache))
	// 		logger.Cache.Infow("take from cache", "cost", time.Since(begin).String(), "key", key)
	// 		return nil // Found cache and return.
	// 	}
	// }
	// if err != redis.ErrKeyNotExists {
	// 	logger.Cache.Error(err)
	// 	return err
	// }
	// // Not Found cache, continue query database.
	// ===========================
	// ===== END redis cache =====
	// ===========================

QUERY:
	var empty M // call nil value M will cause panic.
	// Invoke model hook: GetBefore.
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_BEFORE, span, func(spanCtx context.Context) error {
			return dest.GetBefore(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// if err = db.db.Take(dest).Error; err != nil {
	tableName := db.m.GetTableName()
	if len(db.tableName) > 0 {
		tableName = db.tableName
	}
	if err = db.ins.Table(tableName).Take(dest).Error; err != nil {
		return err
	}
	// Invoke model hook: GetAfter.
	if !db.noHook && !reflect.DeepEqual(empty, dest) {
		if err = traceModelHook[M](db.ctx, consts.PHASE_GET_AFTER, span, func(spanCtx context.Context) error {
			return dest.GetAfter(types.NewModelContext(spanCtx))
		}); err != nil {
			return err
		}
	}
	// // Cache the result.
	// if db.enableCache && config.App.RedisConfig.Enable {
	// 	logger.Cache.Infow("take from database", "cost", time.Since(begin).String(), "key", key)
	// 	go func() {
	// 		if err = redis.SetM[M](key, dest); err != nil {
	// 			logger.Cache.Error(err)
	// 		}
	// 	}()

	//
	// }
	if db.enableCache {
		logger.Cache.Infow("take from database", "cost", util.FormatDurationSmart(time.Since(begin)), "key", key)
		_ = cache.Cache[M]().WithContext(ctx).Set(key, dest, config.App.Cache.Expiration)
	}
	return nil
}
