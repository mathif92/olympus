import { handler } from './handler'

let raw = ''
process.stdin.setEncoding('utf8')
process.stdin.on('data', (c) => { raw += c })
process.stdin.on('end', async () => {
  try {
    const event = JSON.parse(raw || 'null')
    const result = await handler(event)
    process.stdout.write(JSON.stringify(result))
  } catch (err) {
    console.error(err instanceof Error && err.stack ? err.stack : String(err))
    process.exit(1)
  }
})
