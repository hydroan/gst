package dcache

/*
分布式缓存如何使用:
1. 如果缓存只在当前节点使用, 请使用 NewLocalCache
	方法: Set/Get/Delete
2. 如果缓存状态需要跨节点同步, 请使用 NewDistributedCache
	方法: Set/Get/Delete
3. 如果缓存状态需要跨节点同步, 并且需要将缓存同步到 redis 中, 请使用 NewDistributedCache
	方法: SetWithSync/GetWithSync/DeleteWithSync
*/
