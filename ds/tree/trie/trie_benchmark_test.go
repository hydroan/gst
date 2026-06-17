package trie_test

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/ds/tree/trie"
)

var stringKeys [1000]string // random string keys
// var stringKeys [100000]string // random string keys
const bytesPerKey = 30

func init() {
	// string keys
	for i := range stringKeys {
		key := make([]byte, bytesPerKey)
		if _, err := rand.Read(key); err != nil {
			panic("error generating random byte slice")
		}
		stringKeys[i] = string(key)
	}
}

func createTrie1(b *testing.B, safe bool) *trie.Trie[rune, int] {
	b.Helper()
	var t *trie.Trie[rune, int]
	var err error
	if safe {
		t, err = trie.New(trie.WithSafe[rune, int]())
	} else {
		t, err = trie.New[rune, int]()
	}
	if err != nil {
		b.Fatalf("failed to create trie: %v", err)
	}
	return t
}

func createTrie2(b *testing.B, size int, safe bool) *trie.Trie[rune, int] {
	b.Helper()
	var t *trie.Trie[rune, int]
	var err error
	if safe {
		t, err = trie.New(trie.WithSafe[rune, int]())
	} else {
		t, err = trie.New[rune, int]()
	}
	for i := range size {
		t.Put([]rune(stringKeys[i%len(stringKeys)]), i)
	}
	if err != nil {
		b.Fatalf("failed to create trie: %v", err)
	}
	return t
}

func benchmark(b *testing.B, sizes []int, do func(t *trie.Trie[rune, int])) {
	b.Helper()
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("single unsafe", func(b *testing.B) {
				t := createTrie2(b, size, false)
				b.ResetTimer()
				for range b.N {
					do(t)
				}
			})
			b.Run("single safe", func(b *testing.B) {
				t := createTrie2(b, size, true)
				b.ResetTimer()
				for range b.N {
					do(t)
				}
			})

			b.Run("concur safe", func(b *testing.B) {
				t := createTrie2(b, size, true)
				b.ResetTimer()
				b.RunParallel(func(p *testing.PB) {
					for p.Next() {
						do(t)
					}
				})
			})
		})
	}
}

func BenchmarkTrie_Put(b *testing.B) {
	b.Run("single unsafe", func(b *testing.B) {
		trie := createTrie1(b, false)
		b.ResetTimer()
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
	})

	b.Run("single safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		b.ResetTimer()
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
	})

	b.Run("concurr safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				trie.Put([]rune(strconv.Itoa(int(time.Now().UnixNano()))), 0)
			}
		})
	})
}

func BenchmarkTrie_Get(b *testing.B) {
	b.Run("single unsafe", func(b *testing.B) {
		trie := createTrie1(b, false)
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		for i := range b.N {
			trie.Get([]rune(stringKeys[i%len(stringKeys)]))
		}
	})

	b.Run("single safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		for i := range b.N {
			trie.Get([]rune(stringKeys[i%len(stringKeys)]))
		}
	})

	b.Run("concurr safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		for i := range 100000 {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				trie.Get([]rune(strconv.Itoa(int(time.Now().UnixNano()))))
			}
		})
	})
}

func BenchmarkTrie_Delete(b *testing.B) {
	b.Run("single unsafe", func(b *testing.B) {
		trie := createTrie1(b, false)
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		for i := range b.N {
			trie.Delete([]rune(stringKeys[i%len(stringKeys)]))
		}
	})

	b.Run("single safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		for i := range b.N {
			trie.Delete([]rune(stringKeys[i%len(stringKeys)]))
		}
	})

	b.Run("concurr safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		for i := range 100000 {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				trie.Delete([]rune(strconv.Itoa(int(time.Now().UnixNano()))))
			}
		})
	})
}

func BenchmarkTrie_DeletePrefix(b *testing.B) {
	b.Run("single unsafe", func(b *testing.B) {
		trie := createTrie1(b, false)
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		for i := range b.N {
			trie.DeletePrefix([]rune(stringKeys[i%len(stringKeys)]))
		}
	})

	b.Run("single safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		for i := range b.N {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		for i := range b.N {
			trie.DeletePrefix([]rune(stringKeys[i%len(stringKeys)]))
		}
	})

	b.Run("concurr safe", func(b *testing.B) {
		trie := createTrie1(b, true)
		for i := range 100000 {
			trie.Put([]rune(stringKeys[i%len(stringKeys)]), i)
		}
		b.ResetTimer()
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				trie.DeletePrefix([]rune(strconv.Itoa(int(time.Now().UnixNano()))))
			}
		})
	})
}

func BenchmarkTrie_Keys(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.Keys()
	})
}

func BenchmarkTrie_Values(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.Values()
	})
}

func BenchmarkTrie_KeysValues(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.KeysValues()
	})
}

func BenchmarkTrie_PrefixKeys(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.PrefixKeys([]rune("a"))
	})
}

func BenchmarkTrie_PrefixValues(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.PrefixValues([]rune("a"))
	})
}

func BenchmarkTrie_PrefixKeysValues(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.PrefixKeysValues([]rune("a"))
	})
}

func BenchmarkTrie_Range(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		t.Range(func(r []rune, i int) bool {
			_, _ = r, i
			return true
		})
	})
}

func BenchmarkTrie_Clone(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.Clone()
	})
}

func BenchmarkTrie_LongestPrefix(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_, _, _ = t.LongestPrefix([]rune("a"))
	})
}

func BenchmarkTrie_String(b *testing.B) {
	benchmark(b, []int{1000}, func(t *trie.Trie[rune, int]) {
		_ = t.String()
	})
}
