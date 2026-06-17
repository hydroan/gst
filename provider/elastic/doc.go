package elastic

/*
Elasticsearch 布尔查询（Bool Query）说明

1. bool（布尔查询）
   - 作用：用于组合多个查询条件
   - 包含：must、must_not、should、filter 四种子句
   - 特点：每种子句可以包含任意多个查询条件
   - 示例：组合多个查询条件，如 "且" "或" "非" 的组合

2. must（必须匹配）
   - 作用：文档必须匹配这些条件，类似 "AND"
   - 特点：影响文档的相关性得分
   - 计分：参与计算文档的相关性得分
   - 示例：必须是直聊 AND 必须包含某关键字
          must: [
              { "term": { "chat_type.keyword": "direct" } },
              { "match": { "message_text": "hello" } }
          ]

3. must_not（必须不匹配）
   - 作用：文档必须不匹配这些条件，类似 "NOT"
   - 特点：子句中的任何条件都不能匹配
   - 计分：不参与计算文档的相关性得分
   - 示例：排除某类消息或特定用户
          must_not: [
              { "term": { "type.keyword": "system_message" } }
          ]

4. should（应该匹配）
   - 作用：文档应该匹配这些条件，类似 "OR"
   - 特点：通过 minimum_should_match 控制最小匹配数
   - 计分：匹配的条件会增加文档的相关性得分
   - 示例：消息来自用户A OR 消息发给用户A
          should: [
              { "term": { "message_user_id.keyword": "userA" } },
              { "term": { "message_peer_user_id.keyword": "userA" } }
          ]

5. filter（过滤）
   - 作用：文档必须匹配这些条件，类似 must
   - 特点：不影响相关性得分，常用于范围查询
   - 计分：不参与计算文档的相关性得分
   - 示例：过滤时间范围或特定字段
          filter: [
              {
                  "range": {
                      "created_at": {
                          "gte": "2023-01-01",
                          "lte": "2023-12-31"
                      }
                  }
              }
          ]

使用建议：
1. 精确过滤用 filter，性能better：不计算相关性，可缓存
2. 全文搜索用 must：需要计算相关性得分
3. 可选条件用 should：灵活控制匹配程度
4. 排除条件用 must_not：优化搜索结果

常见组合示例：
1. 直聊消息搜索：
   - must：消息类型为直聊
   - should：发送者是A或接收者是A
   - filter：时间范围
   - must_not：排除系统消息

2. 群聊消息搜索：
   - must：消息类型为群聊
   - must：群ID匹配
   - should：包含关键词
   - filter：时间范围

注意事项：
1. should 配合 minimum_should_match 使用
2. filter 优先于 must，性能更好
3. must_not 和 filter 不计算得分
4. 嵌套的布尔查询注意层级关系
*/
