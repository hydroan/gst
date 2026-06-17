package trie

/*
使用

1. 存储字符串
trie := New[rune, int]()
trie.Put([]rune("cat"), 1)
trie.Put([]rune("car"), 2)

	    root
	   /
	  c
	  |
	  a
	 / \
	t   r

2. 存储字节序列：
trie := New[byte, string]()
trie.Put([]byte{192, 168, 1, 0}, "network1")
trie.Put([]byte{192, 168, 1, 1}, "network2")


3. 存储任意序列：
type PathNode struct {
    ID int
    Name string
}

trie := New[PathNode, string]()
path := []PathNode{
    {1, "root"},
    {2, "users"},
    {3, "docs"},
}
trie.Put(path, "document location")
*/

/*

应用

1.前缀搜索
// 查找所有以 "ca" 开头的单词
prefix := []rune("ca")
matches := trie.PrefixSearch(prefix) // 找到 "cat", "car"


2.自动补全
// 根据输入前缀提供补全建议
suggestions := trie.AutoComplete([]rune("c")) // 返回 "cat", "car"


3.IP路由查找
// IP地址路由表查找
routingTrie := New[byte, string]()
routingTrie.Put([]byte{192, 168, 0, 0}, "network1")


4.字典树
dictTrie := New[rune, string]()
dictTrie.Put([]rune("hello"), "greeting")
dictTrie.Put([]rune("help"), "assistance")

*/
