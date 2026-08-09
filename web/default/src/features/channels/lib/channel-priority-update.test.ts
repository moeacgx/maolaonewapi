import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CHANNEL_PRIORITY_UPDATE_DELAY_MS,
  createChannelPriorityUpdateScheduler,
} from './channel-priority-update.ts'

function createFakeTimers() {
  const pending = new Map<number, () => void>()
  let nextId = 1

  return {
    timers: {
      setTimeout: (callback: () => void, delay: number) => {
        assert.equal(delay, CHANNEL_PRIORITY_UPDATE_DELAY_MS)
        const id = nextId++
        pending.set(id, callback)
        return id
      },
      clearTimeout: (id: number) => {
        pending.delete(id)
      },
    },
    fireAll() {
      const callbacks = [...pending.values()]
      pending.clear()
      for (const callback of callbacks) callback()
    },
    get pendingCount() {
      return pending.size
    },
  }
}

describe('channel priority update scheduler', () => {
  test('coalesces rapid schedules into one update with the latest value', () => {
    const fake = createFakeTimers()
    const updates: number[] = []
    const scheduler = createChannelPriorityUpdateScheduler(
      (value) => updates.push(value),
      fake.timers
    )

    scheduler.schedule(1)
    scheduler.schedule(2)
    scheduler.schedule(3)
    assert.deepEqual(updates, [])
    assert.equal(fake.pendingCount, 1)

    fake.fireAll()
    assert.deepEqual(updates, [3])
  })

  test('flush commits pending values immediately and preserves zero', () => {
    const fake = createFakeTimers()
    const updates: number[] = []
    const scheduler = createChannelPriorityUpdateScheduler(
      (value) => updates.push(value),
      fake.timers
    )

    scheduler.schedule(0)
    scheduler.flush()
    assert.deepEqual(updates, [0])
    assert.equal(fake.pendingCount, 0)

    fake.fireAll()
    assert.deepEqual(updates, [0])
  })
})
