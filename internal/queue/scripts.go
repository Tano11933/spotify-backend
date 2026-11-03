package queue

import "github.com/redis/go-redis/v9"

// Redis time avoids early expiry when workers have different system clocks.
const redisNow = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
`

var reserveScript = redis.NewScript(redisNow + `
local raw = redis.call('LINDEX', KEYS[1], 0)
if not raw then return nil end
local ok, delivery = pcall(cjson.decode, raw)
local job = delivery
if ok and type(delivery) == 'table' and type(delivery.raw_job) == 'string' then
  ok, job = pcall(cjson.decode, delivery.raw_job)
else
  delivery = {raw_job = raw, attempt = ok and type(job) == 'table' and job.attempt or 0,
    last_error = ok and type(job) == 'table' and job.last_error or ''}
end
if not ok or type(job) ~= 'table' or type(job.id) ~= 'string'
  or type(job.type) ~= 'string' or type(delivery.attempt) ~= 'number'
  or type(job.enqueued_at) ~= 'string' then
  redis.call('RPUSH', KEYS[5], raw)
  redis.call('LPOP', KEYS[1])
  return 'invalid'
end
delivery.attempt = delivery.attempt + 1
raw = cjson.encode(delivery)
redis.call('HSET', KEYS[2], ARGV[1], raw)
redis.call('ZADD', KEYS[3], now + tonumber(ARGV[2]), ARGV[1])
redis.call('LPOP', KEYS[1])
return raw
`)

var promoteScript = redis.NewScript(redisNow + `
local jobs = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', now, 'LIMIT', 0, 100)
for _, raw in ipairs(jobs) do
  redis.call('RPUSH', KEYS[1], raw)
  redis.call('ZREM', KEYS[4], raw)
end
return #jobs
`)

var finishScript = redis.NewScript(redisNow + `
local raw = redis.call('HGET', KEYS[2], ARGV[1])
local deadline = redis.call('ZSCORE', KEYS[3], ARGV[1])
if not raw or not deadline or tonumber(deadline) <= now then return 0 end
if ARGV[2] == 'renew' then
  redis.call('ZADD', KEYS[3], now + tonumber(ARGV[4]), ARGV[1])
  return 1
end
if ARGV[2] ~= 'ack' then
  local job = cjson.decode(raw)
  job.last_error = ARGV[3]
  raw = cjson.encode(job)
  if ARGV[2] == 'dead' or job.attempt >= tonumber(ARGV[5]) then
    redis.call('RPUSH', KEYS[5], raw)
  else
    local delay = math.min(tonumber(ARGV[6]) * 2 ^ (job.attempt - 1), tonumber(ARGV[7]))
    redis.call('ZADD', KEYS[4], now + delay, raw)
  end
end
redis.call('HDEL', KEYS[2], ARGV[1])
redis.call('ZREM', KEYS[3], ARGV[1])
return 1
`)
