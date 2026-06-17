package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider/mongo"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var RunOrDie = util.RunOrDie

func TestMongo(t *testing.T) {
	config.SetConfigFile("../examples/myproject/config.ini")
	RunOrDie(bootstrap.Bootstrap)
	defer mongo.Close()
	require.NoError(t, mongo.Health())

	dbName := "test"
	collName := "users"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coll, err := mongo.Collection(dbName, collName)
	require.NoError(t, err)

	// clean
	_, err = coll.DeleteMany(ctx, bson.M{})
	require.NoError(t, err)

	t.Run("insert one", func(t *testing.T) {
		res, err := coll.InsertOne(ctx, bson.M{
			"name": "test1",
			"age":  20,
		})
		require.NoError(t, err)
		assert.NotNil(t, res.InsertedID)
	})
	t.Run("insert many", func(t *testing.T) {
		docs := []any{
			bson.M{"name": "test2", "age": 21},
			bson.M{"name": "test3", "age": 22},
		}
		res, err := coll.InsertMany(ctx, docs)
		require.NoError(t, err)
		assert.NotNil(t, 3, len(res.InsertedIDs))
	})

	t.Run("find one", func(t *testing.T) {
		var res bson.M
		err := coll.FindOne(ctx, bson.M{"name": "test1"}).Decode(&res)
		require.NoError(t, err)
		assert.Equal(t, "test1", res["name"])
	})
	t.Run("find many", func(t *testing.T) {
		cursor, err := coll.Find(ctx, bson.M{"age": bson.M{"$gte": 20}})
		require.NoError(t, err)
		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("update one", func(t *testing.T) {
		update := bson.M{"$set": bson.M{"age": 25}}
		updateRes, err := coll.UpdateOne(
			ctx,
			bson.M{"name": "test1"},
			update,
		)
		require.NoError(t, err)
		assert.Equal(t, int64(1), updateRes.ModifiedCount)
	})
	t.Run("update many", func(t *testing.T) {
		updateMany, err := coll.UpdateMany(
			ctx,
			bson.M{"age": bson.M{"$lt": 25}},
			bson.M{"$inc": bson.M{"age": 1}},
		)
		require.NoError(t, err)
		assert.Equal(t, int64(2), updateMany.ModifiedCount)
	})

	t.Run("delete one", func(t *testing.T) {
		deleteRes, err := coll.DeleteOne(ctx, bson.M{"name": "test1"})
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleteRes.DeletedCount)
	})
	t.Run("delete many", func(t *testing.T) {
		deleteMany, err := coll.DeleteMany(ctx, bson.M{"age": bson.M{"$gt": 20}})
		require.NoError(t, err)
		assert.Equal(t, int64(2), deleteMany.DeletedCount)
	})
}
