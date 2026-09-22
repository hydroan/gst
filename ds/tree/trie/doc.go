package trie

/*
Usage

1. Storing strings
trie, _ := New[rune, int]()
trie.Put([]rune("cat"), 1)
trie.Put([]rune("car"), 2)

	    root
	   /
	  c
	  |
	  a
	 / \
	t   r

2. Storing byte sequences:
trie, _ := New[byte, string]()
trie.Put([]byte{192, 168, 1, 0}, "network1")
trie.Put([]byte{192, 168, 1, 1}, "network2")


3. Storing arbitrary sequences:
type PathNode struct {
    ID int
    Name string
}

trie, _ := New[PathNode, string]()
path := []PathNode{
    {1, "root"},
    {2, "users"},
    {3, "docs"},
}
trie.Put(path, "document location")
*/

/*

Applications

1. Prefix search
// find every word starting with "ca"
prefix := []rune("ca")
matches := trie.PrefixKeys(prefix) // finds "cat", "car"


2. Auto completion
// suggest completions for the entered prefix
suggestions := trie.PrefixKeys([]rune("c")) // returns "cat", "car"


3. IP routing lookup
// look up an IP address in a routing table
routingTrie, _ := New[byte, string]()
routingTrie.Put([]byte{192, 168, 0, 0}, "network1")


4. Dictionary
dictTrie, _ := New[rune, string]()
dictTrie.Put([]rune("hello"), "greeting")
dictTrie.Put([]rune("help"), "assistance")

*/
