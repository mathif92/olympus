require 'json'
require_relative 'handler'

event = JSON.parse(STDIN.read)
result = handler(event)
STDOUT.write(JSON.generate(result))
