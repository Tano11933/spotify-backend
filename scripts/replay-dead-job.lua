-- KEYS: dead-letter list, ready list. Replays only the oldest valid job.
local raw = redis.call('LINDEX', KEYS[1], 0)
if not raw then return nil end
local ok, delivery = pcall(cjson.decode, raw)
local job = delivery
if ok and type(delivery) == 'table' and type(delivery.raw_job) == 'string' then
  ok, job = pcall(cjson.decode, delivery.raw_job)
else
  delivery = {raw_job = raw}
end
if not ok or type(job) ~= 'table' or type(job.id) ~= 'string'
  or type(job.type) ~= 'string' or type(job.enqueued_at) ~= 'string' then
  return redis.error_reply('invalid dead-letter job; inspect it before replay')
end
delivery.attempt = 0
delivery.last_error = ''
redis.call('RPUSH', KEYS[2], cjson.encode(delivery))
redis.call('LPOP', KEYS[1])
return job.id
