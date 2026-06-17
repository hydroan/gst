package trie_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hydroan/gst/ds/tree/trie"
	"github.com/stretchr/testify/assert"
)

var (
	wordsA = []string{
		"a", "an", "and", "ant",
		"app", "apple", "apply", "apartment",
		"about", "above", "abroad",
		"after", "afternoon", "afterwards",
	}
	wordsB = []string{
		"bad", "bag", "baggage",
		"back", "background", "backup",
		"bank", "banker", "banking",
		"book", "bookmark", "bookkeeper",
	}
	wordsC = []string{
		"cat", "catch", "cattle",
		"computer", "compute", "computing",
		"car", "card", "cardboard",
		"care", "careful", "carefully",
	}
)

func TestTrie(t *testing.T) {
	// t, err := trie.New(trie.WithSafe[rune, int]())
	tt, err := trie.New(
		trie.WithSafe[rune, string](),
		trie.WithKeyFormatter(func(k rune, v string, count int, hasValue bool) string {
			if count > 1 {
				return fmt.Sprintf("%s(%d)", string(k), count)
			}
			return string(k)
		}),
	)
	if err != nil {
		t.Fatalf("failed to create trie: %v", err)
	}

	expectedKeys := [][]rune{
		[]rune("bad"),
		[]rune("bag"),
		[]rune("baggage"),
		[]rune("back"),
		[]rune("background"),
		[]rune("backup"),
		[]rune("bank"),
		[]rune("banker"),
		[]rune("banking"),
	}
	expectedAllKeys := [][]rune{
		[]rune("bad"),
		[]rune("bag"),
		[]rune("baggage"),
		[]rune("back"),
		[]rune("background"),
		[]rune("backup"),
		[]rune("bank"),
		[]rune("banker"),
		[]rune("banking"),
		[]rune("book"),
		[]rune("bookmark"),
		[]rune("bookkeeper"),
	}
	expectedValues := []string{"bag", "baggage", "back", "background", "backup", "bank", "banker", "banking", "bad"}
	expectedAllValues := []string{"bad", "bag", "baggage", "back", "background", "backup", "bank", "banker", "banking", "book", "bookmark", "bookkeeper"}

	for _, word := range wordsB {
		tt.Put([]rune(word), word)
	}
	// put wordsB twice
	for _, word := range wordsB {
		tt.Put([]rune(word), word)
	}
	t.Run("Keys", func(t *testing.T) {
		assert.ElementsMatch(t, expectedAllKeys, tt.Keys())
	})
	t.Run("Values", func(t *testing.T) {
		assert.ElementsMatch(t, expectedAllValues, tt.Values())
	})
	t.Run("keysValues", func(t *testing.T) {
		keysValues := tt.KeysValues()
		keys := make([][]rune, 0, len(keysValues))
		values := make([]string, 0, len(keysValues))
		for _, kv := range keysValues {
			keys = append(keys, kv.Keys)
			values = append(values, kv.Value)
		}
		assert.ElementsMatch(t, tt.Keys(), keys)
		assert.ElementsMatch(t, tt.Values(), values)
	})
	for _, work := range wordsA {
		tt.Put([]rune(work), work)
	}
	for _, word := range wordsC {
		tt.Put([]rune(word), word)
	}
	keys := tt.PrefixKeys([]rune("ba"))
	values := tt.PrefixValues([]rune("ba"))
	t.Run("PrefixKeys", func(t *testing.T) {
		assert.ElementsMatch(t, expectedKeys, keys)
	})
	t.Run("PrefixValues", func(t *testing.T) {
		assert.ElementsMatch(t, expectedValues, values)
	})

	t.Run("PrefixKeysValue", func(t *testing.T) {
		keysValues := tt.PrefixKeysValues([]rune("ba"))
		assert.Len(t, keysValues, len(expectedKeys))
		keys := make([][]rune, 0, len(keysValues))
		values := make([]string, 0, len(keysValues))
		for _, keysValue := range keysValues {
			keys = append(keys, keysValue.Keys)
			values = append(values, keysValue.Value)
		}
		assert.ElementsMatch(t, expectedKeys, keys)
		assert.ElementsMatch(t, expectedValues, values)
	})

	t.Run("PrefixCount", func(t *testing.T) {
		assert.Equal(t, len(wordsC), tt.PrefixCount([]rune("c")))
	})

	t.Run("Basic", func(t *testing.T) {
		val, ok := tt.Delete([]rune("c"))
		assert.Equal(t, len(wordsC), tt.PrefixCount([]rune("c")))
		assert.Equal(t, len(wordsA)+len(wordsB)+len(wordsC), tt.Size())
		assert.False(t, tt.IsEmpty())
		assert.Empty(t, val)
		assert.False(t, ok)

		val, ok = tt.Delete([]rune("cat"))
		assert.Equal(t, len(wordsC)-1, tt.PrefixCount([]rune("c")))
		assert.Equal(t, len(wordsA)+len(wordsB)+len(wordsC)-1, tt.Size())
		assert.False(t, tt.IsEmpty())
		assert.Equal(t, "cat", val)
		assert.True(t, ok)

		ok = tt.Put([]rune("car2"), "car2")
		assert.True(t, ok)
		tt.Delete([]rune("car2"))
		tt.Put([]rune("car"), "car_modified")
		val, ok = tt.Get([]rune("car"))
		assert.Equal(t, len(wordsC)-1, tt.PrefixCount([]rune("c")))
		assert.Equal(t, len(wordsA)+len(wordsB)+len(wordsC)-1, tt.Size())
		assert.False(t, tt.IsEmpty())
		assert.Equal(t, "car_modified", val)
		assert.True(t, ok)
	})

	t.Run("Clone", func(t *testing.T) {
		clone := tt.Clone()
		assert.Equal(t, tt.Size(), clone.Size())
		assert.Equal(t, tt.IsEmpty(), clone.IsEmpty())
		assert.Equal(t, tt.PrefixCount([]rune("a")), clone.PrefixCount([]rune("a")))
		assert.Equal(t, tt.PrefixCount([]rune("c")), clone.PrefixCount([]rune("c")))

		clone.Put([]rune("a_notexists"), "a_notexists")
		assert.NotEqual(t, tt.Size(), clone.Size())
		assert.NotEqual(t, tt.PrefixCount([]rune("a")), clone.PrefixCount([]rune("a")))
		assert.Equal(t, tt.PrefixCount([]rune("c")), clone.PrefixCount([]rune("c")))

		clone.Put([]rune("c_notexists"), "c_notexists")
		assert.NotEqual(t, tt.Size(), clone.Size())
		assert.NotEqual(t, tt.PrefixCount([]rune("a")), clone.PrefixCount([]rune("a")))
		assert.NotEqual(t, tt.PrefixCount([]rune("c")), clone.PrefixCount([]rune("c")))
	})

	t.Run("Clear", func(t *testing.T) {
		tt.Clear()
		assert.Zero(t, tt.Size())
		assert.True(t, tt.IsEmpty())
	})

	t.Run("Range", func(t *testing.T) {
		tt, err = trie.New[rune, string]()
		if err != nil {
			t.Fatalf("failed to create trie: %v", err)
		}
		for _, word := range wordsB {
			tt.Put([]rune(word), word)
		}

		keys := make([]string, 0, tt.Size())
		values := make([]string, 0, tt.Size())
		tt.Range(func(r []rune, s string) bool {
			keys = append(keys, string(r))
			values = append(values, s)
			return true
		})
		assert.ElementsMatch(t, wordsB, keys)
		assert.ElementsMatch(t, wordsB, values)
	})

	t.Run("DeletePrefix", func(t *testing.T) {
		tt.Clear()
		for _, word := range wordsA {
			tt.Put([]rune(word), word)
		}
		for _, word := range wordsB {
			tt.Put([]rune(word), word)
		}
		assert.Equal(t, len(wordsA)+len(wordsB), tt.Size())
		count := tt.DeletePrefix([]rune("a"))
		assert.Equal(t, len(wordsA), count)
		assert.Equal(t, len(wordsB), tt.Size())

		for _, word := range wordsC {
			tt.Put([]rune(word), word)
		}
		assert.Equal(t, len(wordsB)+len(wordsC), tt.Size())

		count = tt.DeletePrefix([]rune("b"))
		assert.Equal(t, len(wordsB), count)
		assert.Equal(t, len(wordsC), tt.Size())
	})

	t.Run("JSON", func(t *testing.T) {
		tt.Clear()
		for _, w := range wordsA {
			tt.Put([]rune(w), w)
		}
		for _, w := range wordsB {
			tt.Put([]rune(w), w)
		}
		for _, w := range wordsC {
			tt.Put([]rune(w), w)
		}

		b, err := json.Marshal(tt)
		if err != nil {
			t.Fatal(err)
		}
		var tt2 trie.Trie[rune, string]
		err = json.Unmarshal(b, &tt2)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, tt.Size(), tt2.Size())
		assert.Equal(t, tt.IsEmpty(), tt2.IsEmpty())
		assert.Equal(t, tt.PrefixCount([]rune("a")), tt2.PrefixCount([]rune("a")))
		assert.Equal(t, tt.PrefixCount([]rune("b")), tt2.PrefixCount([]rune("b")))
		assert.Equal(t, tt.PrefixCount([]rune("c")), tt2.PrefixCount([]rune("c")))

		assert.Equal(t, len(wordsA), tt2.DeletePrefix([]rune("a")))
		assert.Equal(t, 0, tt2.PrefixCount([]rune("a")))
		assert.Equal(t, len(wordsB)+len(wordsC), tt2.Size())

		assert.Equal(t, len(wordsB), tt2.DeletePrefix([]rune("b")))
		assert.Equal(t, 0, tt2.PrefixCount([]rune("b")))
		assert.Equal(t, len(wordsC), tt2.Size())
	})
}

func TestTrie_LongestPrefix(t *testing.T) {
	{
		// Router matching example
		router, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		router.Put([]rune("/api"), "API Handler")
		router.Put([]rune("/api/users"), "Users Handler")
		router.Put([]rune("/api/users/profiles"), "Profiles Handler")

		path := []rune("/api/users/profiles/123")
		prefix, handler, ok := router.LongestPrefix(path)
		assert.Equal(t, "/api/users/profiles", string(prefix))
		assert.Equal(t, "Profiles Handler", handler)
		assert.True(t, ok)
	}

	{
		// IP routing example
		ipRouter, err := trie.New[byte, string]()
		if err != nil {
			t.Fatal(err)
		}
		ipRouter.Put([]byte{192, 168}, "Local Network")
		ipRouter.Put([]byte{192, 168, 1}, "Subnet 1")
		ip := []byte{192, 168, 1, 100}
		prefix, network, ok := ipRouter.LongestPrefix(ip)
		assert.Equal(t, []byte{192, 168, 1}, prefix)
		assert.Equal(t, "Subnet 1", network)
		assert.True(t, ok)
	}

	{
		// Word segmentation example
		dict, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		dict.Put([]rune("inter"), "INTER")
		dict.Put([]rune("internal"), "INTERNAL")
		dict.Put([]rune("international"), "INTERNATIONAL")
		word := []rune("internationally")
		prefix, meaning, ok := dict.LongestPrefix(word)
		assert.Equal(t, []rune("international"), prefix)
		assert.Equal(t, "INTERNATIONAL", meaning)
		assert.True(t, ok)
	}
}

func TestTrie_PathAncestors(t *testing.T) {
	{
		// Router hierarchy example
		router, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		router.Put([]rune("/"), "Root Handler")
		router.Put([]rune("/api"), "API Handler")
		router.Put([]rune("/api/users"), "Users Handler")
		router.Put([]rune("/api/users/profiles"), "Profiles Handler")

		path := []rune("/api/users/profiles")
		ancestors := router.PathAncestors(path)
		assert.Len(t, ancestors, 4)

		// Check root
		assert.Equal(t, []rune("/"), ancestors[0].Keys)
		assert.Equal(t, "Root Handler", ancestors[0].Value)

		// Check /api
		assert.Equal(t, []rune("/api"), ancestors[1].Keys)
		assert.Equal(t, "API Handler", ancestors[1].Value)

		// Check /api/users
		assert.Equal(t, []rune("/api/users"), ancestors[2].Keys)
		assert.Equal(t, "Users Handler", ancestors[2].Value)

		// Check /api/users/profiles
		assert.Equal(t, []rune("/api/users/profiles"), ancestors[3].Keys)
		assert.Equal(t, "Profiles Handler", ancestors[3].Value)
	}

	{
		// File system hierarchy example
		fs, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		fs.Put([]rune("home"), "Home Directory")
		fs.Put([]rune("home/user"), "User Directory")
		fs.Put([]rune("home/user/docs"), "Documents Directory")

		path := []rune("home/user/docs/file.txt")
		ancestors := fs.PathAncestors(path)
		assert.Len(t, ancestors, 3)

		assert.Equal(t, []rune("home"), ancestors[0].Keys)
		assert.Equal(t, "Home Directory", ancestors[0].Value)

		assert.Equal(t, []rune("home/user"), ancestors[1].Keys)
		assert.Equal(t, "User Directory", ancestors[1].Value)

		assert.Equal(t, []rune("home/user/docs"), ancestors[2].Keys)
		assert.Equal(t, "Documents Directory", ancestors[2].Value)
	}

	{
		// Empty path test
		trie, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		ancestors := trie.PathAncestors(nil)
		assert.Nil(t, ancestors)
	}

	{
		// Non-existent path test
		trie, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		trie.Put([]rune("abc"), "ABC")
		ancestors := trie.PathAncestors([]rune("xyz"))
		assert.Empty(t, ancestors)
	}

	{
		// Partial path with gaps test
		trie, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		trie.Put([]rune("a"), "A")
		// Skip "ab" - no value
		trie.Put([]rune("abc"), "ABC")

		ancestors := trie.PathAncestors([]rune("abc"))
		assert.Len(t, ancestors, 2)

		assert.Equal(t, []rune("a"), ancestors[0].Keys)
		assert.Equal(t, "A", ancestors[0].Value)

		assert.Equal(t, []rune("abc"), ancestors[1].Keys)
		assert.Equal(t, "ABC", ancestors[1].Value)
	}

	{
		// Root with value test
		trie, err := trie.New[rune, string]()
		if err != nil {
			t.Fatal(err)
		}
		// Put empty slice for root
		trie.Put([]rune{}, "Root")
		trie.Put([]rune("a"), "A")

		ancestors := trie.PathAncestors([]rune("a"))
		assert.Len(t, ancestors, 2)

		assert.Equal(t, []rune{}, ancestors[0].Keys)
		assert.Equal(t, "Root", ancestors[0].Value)

		assert.Equal(t, []rune("a"), ancestors[1].Keys)
		assert.Equal(t, "A", ancestors[1].Value)
	}
}
