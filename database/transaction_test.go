package database_test

import (
	"errors"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestDatabaseTransaction(t *testing.T) {
	defer cleanupTestData()

	flag := 0
	users := make([]*TestUser, 0)

	err := database.Database[*TestUser](nil).Transaction(nil)
	require.ErrorIs(t, err, database.ErrNilTransactionFunc)

	// Test Transaction - transaction success
	// Transaction automatically injects txDB, no need for WithTx
	err = database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
		// No need to call WithTx - txDB already has transaction context
		return txDB.Create(ul...)
	})
	require.NoError(t, err, "transaction should succeed")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Len(t, users, 3, "should have 3 records after successful transaction")

	// Verify created data integrity
	var foundU1, foundU2, foundU3 bool
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
			require.Equal(t, u1.Name, u.Name, "u1 name should match")
		case u2.ID:
			foundU2 = true
			require.Equal(t, u2.Name, u.Name, "u2 name should match")
		case u3.ID:
			foundU3 = true
			require.Equal(t, u3.Name, u.Name, "u3 name should match")
		}
	}
	require.True(t, foundU1 && foundU2 && foundU3, "all users should be found")

	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))

	// Test Transaction - transaction failed with rollback
	// Rollback will execute if transaction failed, so resources will not be created
	errTest := errors.New("test error")
	err = database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
		require.NoError(t, txDB.Create(ul...))
		return errTest
	})
	require.Error(t, err, "transaction should fail")
	require.ErrorIs(t, err, errTest)
	require.Contains(t, err.Error(), "test error", "error should contain test error message")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Empty(t, users, "should have 0 records after rollback")

	// Test Transaction - multiple operations in transaction
	err = database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
		// Create users
		if txErr := txDB.Create(u1); txErr != nil {
			return txErr
		}
		// Update user in the same transaction
		u1.Name = "user1_updated"
		if txErr := txDB.Update(u1); txErr != nil {
			return txErr
		}
		// UpdateByID in the same transaction
		return txDB.UpdateByID(u1.ID, "age", 25)
	})
	require.NoError(t, err, "transaction should succeed")

	// Verify the updates were committed
	u := new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).Get(u, u1.ID))
	require.Equal(t, "user1_updated", u.Name, "name should be updated")
	require.Equal(t, 25, u.Age, "age should be updated")

	require.NoError(t, database.Database[*TestUser](nil).Delete(u1))

	// Test Transaction - transaction success with custom rollback function
	// Rollback function should not execute if transaction succeeds
	err = database.Database[*TestUser](nil).WithRollback(func() {
		flag++
	}).Transaction(func(txDB types.Database[*TestUser]) error {
		return txDB.Create(ul...)
	})
	require.NoError(t, err, "transaction should succeed")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Len(t, users, 3, "should have 3 records after successful transaction")
	require.Equal(t, 0, flag, "rollback function should not be called on success")

	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))

	// Test Transaction - transaction failed with custom rollback function
	// Rollback function should execute if transaction fails
	err = database.Database[*TestUser](nil).WithRollback(func() {
		flag++
	}).Transaction(func(txDB types.Database[*TestUser]) error {
		require.NoError(t, txDB.Create(ul...))
		return errors.New("test error")
	})
	require.Error(t, err, "transaction should fail")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Empty(t, users, "should have 0 records after rollback")
	require.Equal(t, 1, flag, "rollback function should be called on failure")

	// Test Transaction - with query options (WithLock, WithQuery, etc.)
	flag = 0
	require.NoError(t, database.Database[*TestUser](nil).Create(u1))
	err = database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
		lockedUser := new(TestUser)
		// Test WithLock works in transaction
		if lockErr := txDB.WithLock(consts.LockUpdate).Get(lockedUser, u1.ID); lockErr != nil {
			return lockErr
		}
		lockedUser.Name = "locked_update"
		return txDB.Update(lockedUser)
	})
	require.NoError(t, err, "transaction with lock should succeed")
	u = new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).Get(u, u1.ID))
	require.Equal(t, "locked_update", u.Name, "name should be updated")

	require.NoError(t, database.Database[*TestUser](nil).Delete(u1))
}

func TestDatabaseTransactionFunc(t *testing.T) {
	defer cleanupTestData()

	flag := 0
	users := make([]*TestUser, 0)

	err := database.Database[*TestUser](nil).TransactionFunc(nil)
	require.ErrorIs(t, err, database.ErrNilTransactionFunc)

	// Test TransactionFunc - transaction success
	err = database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
		return nil
	})
	require.NoError(t, err, "transaction should succeed")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Len(t, users, 3, "should have 3 records after successful transaction")
	require.Equal(t, 0, flag, "rollback function should not be called on success")

	// Verify created data integrity
	var foundU1, foundU2, foundU3 bool
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
			require.Equal(t, u1.Name, u.Name, "u1 name should match")
		case u2.ID:
			foundU2 = true
			require.Equal(t, u2.Name, u.Name, "u2 name should match")
		case u3.ID:
			foundU3 = true
			require.Equal(t, u3.Name, u.Name, "u3 name should match")
		}
	}
	require.True(t, foundU1 && foundU2 && foundU3, "all users should be found")

	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))

	// Test TransactionFunc - transaction failed with rollback
	// Rollback will execute if transaction failed, so resource will not be created
	errTest := errors.New("test error")
	err = database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
		return errTest
	})
	require.Error(t, err, "transaction should fail")
	require.ErrorIs(t, err, errTest)
	require.Contains(t, err.Error(), "test error", "error should contain test error message")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Empty(t, users, "should have 0 records after rollback")
	require.Equal(t, 0, flag, "rollback function should not be called without WithRollback")

	// Test TransactionFunc - incorrect use (not using WithTx)
	// Rollback will not execute if not using WithTx option, so resources will be created outside transaction
	err = database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).Create(ul...))
		return errors.New("test error")
	})
	require.Error(t, err, "transaction should fail")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Len(t, users, 3, "should have 3 records because Create was not in transaction")
	require.Equal(t, 0, flag, "rollback function should not be called")

	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))

	// Test TransactionFunc - transaction success with custom rollback function
	// Rollback function should not execute if transaction succeeds
	err = database.Database[*TestUser](nil).WithRollback(func() {
		flag++
	}).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
		return nil
	})
	require.NoError(t, err, "transaction should succeed")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Len(t, users, 3, "should have 3 records after successful transaction")
	require.Equal(t, 0, flag, "rollback function should not be called on success")

	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))

	// Test TransactionFunc - transaction failed with custom rollback function
	// Rollback function should execute if transaction fails
	err = database.Database[*TestUser](nil).WithRollback(func() {
		flag++
	}).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
		return errors.New("test error")
	})
	require.Error(t, err, "transaction should fail")
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Empty(t, users, "should have 0 records after rollback")
	require.Equal(t, 1, flag, "rollback function should be called on failure")
}

func TestDatabaseWithTx(t *testing.T) {
	defer func() {
		cleanupTestData()
		_ = database.Database[*TestProduct](nil).Delete()
	}()

	// Transaction success - Create operation
	err := database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
		return nil
	})
	require.NoError(t, err)
	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Len(t, users, 3)
	// Verify data integrity
	var foundU1, foundU2, foundU3 bool
	for _, u := range users {
		switch u.ID {
		case u1.ID:
			foundU1 = true
			require.Equal(t, u1.Name, u.Name)
			require.Equal(t, u1.Age, u.Age)
			require.Equal(t, u1.Email, u.Email)
		case u2.ID:
			foundU2 = true
			require.Equal(t, u2.Name, u.Name)
		case u3.ID:
			foundU3 = true
			require.Equal(t, u3.Name, u.Name)
		}
	}
	require.True(t, foundU1 && foundU2 && foundU3, "all users should be found")

	// Transaction success - Update operation
	require.NoError(t, database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		u1.Name = "user1_updated"
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Update(u1))
		return nil
	}))
	user := new(TestUser)
	require.NoError(t, database.Database[*TestUser](nil).Get(user, u1.ID))
	require.Equal(t, "user1_updated", user.Name)
	u1.Name = "user1" // restore

	// Transaction success - Multiple resource types
	p1 := &TestProduct{Name: "product1", Price: 10.0, Base: model.Base{ID: "p1"}}
	require.NoError(t, database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(u1))
		require.NoError(t, database.Database[*TestProduct](nil).WithTx(tx).Create(p1))
		return nil
	}))
	product := new(TestProduct)
	require.NoError(t, database.Database[*TestProduct](nil).Get(product, p1.ID))
	require.NotNil(t, product)
	require.Equal(t, p1.Name, product.Name)

	// Transaction success - List operation within transaction
	require.NoError(t, database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		txUsers := make([]*TestUser, 0)
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).List(&txUsers))
		require.NotEmpty(t, txUsers, "should find users within transaction")
		return nil
	}))

	// Transaction failed - rollback on error
	require.NoError(t, database.Database[*TestUser](nil).Delete(ul...))
	err = database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
		return errors.New("custom error")
	})
	require.Error(t, err)
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.Empty(t, users, "transaction should be rolled back, no users created")

	// Transaction with chainable methods
	require.NoError(t, database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
		require.NoError(t, database.Database[*TestUser](nil).
			WithTx(tx).
			WithQuery(&TestUser{Name: u1.Name}).
			Create(u1))
		return nil
	}))
	users = make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](nil).List(&users))
	require.GreaterOrEqual(t, len(users), 1, "should find created user")
}

func TestDatabaseWithRollback(t *testing.T) {
	defer cleanupTestData()

	t.Run("TransactionFunc", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			defer cleanupTestData()
			flag := 0
			err := database.Database[*TestUser](nil).WithRollback(func() {
				flag++
			}).TransactionFunc(func(tx any) error {
				require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
				return nil
			})

			require.NoError(t, err)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
			// Transaction success, rollback function should not be called.
			require.Equal(t, 0, flag, "rollback function should not be called when transaction succeeds")
		})

		t.Run("failure", func(t *testing.T) {
			defer cleanupTestData()
			flag := 0
			errTest := errors.New("test error")
			err := database.Database[*TestUser](nil).WithRollback(func() {
				flag++
			}).TransactionFunc(func(tx any) error {
				require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).Create(ul...))
				return errTest
			})

			require.Error(t, err)
			require.ErrorIs(t, err, errTest)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Empty(t, users, "should have 0 records after rollback")
			// Transaction failure, rollback function should be called.
			require.Equal(t, 1, flag, "rollback function should be called when transaction fails")
		})
	})

	t.Run("Transaction", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			defer cleanupTestData()
			flag := 0
			err := database.Database[*TestUser](nil).WithRollback(func() {
				flag++
			}).Transaction(func(txDB types.Database[*TestUser]) error {
				require.NoError(t, txDB.Create(ul...))
				return nil
			})

			require.NoError(t, err)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Len(t, users, 3)
			// Transaction success, rollback function should not be called.
			require.Equal(t, 0, flag, "rollback function should not be called when transaction succeeds")
		})

		t.Run("failure", func(t *testing.T) {
			defer cleanupTestData()
			flag := 0
			errTest := errors.New("test error")
			err := database.Database[*TestUser](nil).WithRollback(func() {
				flag++
			}).Transaction(func(txDB types.Database[*TestUser]) error {
				require.NoError(t, txDB.Create(ul...))
				return errTest
			})

			require.Error(t, err)
			require.ErrorIs(t, err, errTest)
			users := make([]*TestUser, 0)
			require.NoError(t, database.Database[*TestUser](nil).List(&users))
			require.Empty(t, users, "should have 0 records after rollback")
			// Transaction failure, rollback function should be called.
			require.Equal(t, 1, flag, "rollback function should be called when transaction fails")
		})
	})
}

func TestDatabaseWithLock(t *testing.T) {
	defer cleanupTestData()

	t.Run("TransactionFunc", func(t *testing.T) {
		t.Run("with LockUpdate", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
				// Get and lock user with FOR UPDATE
				user := new(TestUser)
				require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).WithLock(consts.LockUpdate).Get(user, u1.ID))
				require.Equal(t, u1.ID, user.ID)
				require.Equal(t, u1.Name, user.Name)

				// Update the locked user
				user.Name = "locked_update"
				return database.Database[*TestUser](nil).WithTx(tx).Update(user)
			})
			require.NoError(t, err)

			// Verify update was successful
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](nil).Get(user, u1.ID))
			require.Equal(t, "locked_update", user.Name)
		})

		t.Run("with LockUpdateNoWait", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
				// Get and lock user with FOR UPDATE NOWAIT
				user := new(TestUser)
				require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).WithLock(consts.LockUpdateNoWait).Get(user, u1.ID))
				require.Equal(t, u1.ID, user.ID)
				return nil
			})
			require.NoError(t, err)
		})

		t.Run("with LockShare", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
				// Get user with FOR SHARE lock
				user := new(TestUser)
				require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).WithLock(consts.LockShare).Get(user, u1.ID))
				require.Equal(t, u1.ID, user.ID)
				return nil
			})
			require.NoError(t, err)
		})

		t.Run("with List", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).TransactionFunc(func(tx any) error {
				// List users with lock
				users := make([]*TestUser, 0)
				require.NoError(t, database.Database[*TestUser](nil).WithTx(tx).WithLock(consts.LockUpdate).List(&users))
				require.Len(t, users, 3)
				return nil
			})
			require.NoError(t, err)
		})
	})

	t.Run("Transaction", func(t *testing.T) {
		t.Run("with LockUpdate", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
				// Get and lock user with FOR UPDATE
				user := new(TestUser)
				require.NoError(t, txDB.WithLock(consts.LockUpdate).Get(user, u1.ID))
				require.Equal(t, u1.ID, user.ID)
				require.Equal(t, u1.Name, user.Name)

				// Update the locked user
				user.Name = "locked_update"
				return txDB.Update(user)
			})
			require.NoError(t, err)

			// Verify update was successful
			user := new(TestUser)
			require.NoError(t, database.Database[*TestUser](nil).Get(user, u1.ID))
			require.Equal(t, "locked_update", user.Name)
		})

		t.Run("with default lock mode", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
				// Get user with default lock (FOR UPDATE)
				user := new(TestUser)
				require.NoError(t, txDB.WithLock().Get(user, u1.ID))
				require.Equal(t, u1.ID, user.ID)
				return nil
			})
			require.NoError(t, err)
		})

		t.Run("with List", func(t *testing.T) {
			defer cleanupTestData()
			setupTestData(t)

			err := database.Database[*TestUser](nil).Transaction(func(txDB types.Database[*TestUser]) error {
				// List users with lock
				users := make([]*TestUser, 0)
				require.NoError(t, txDB.WithLock(consts.LockUpdate).List(&users))
				require.Len(t, users, 3)
				return nil
			})
			require.NoError(t, err)
		})
	})
}
