import { describe, expect, test } from 'bun:test'

import { getRoutingTargetCount } from '../src/features/channels/lib'
import type { Channel } from '../src/features/channels/types'

describe('channel routing target count', () => {
  test('uses the channel count for a regular row', () => {
    const channel = { routing_target_count: 2 } as Channel

    expect(getRoutingTargetCount(channel)).toBe(2)
  })

  test('sums child counts for a tag aggregate row', () => {
    const aggregate = {
      routing_target_count: 99,
      children: [
        { routing_target_count: 2 },
        { routing_target_count: 1 },
        { routing_target_count: 0 },
      ],
    } as Channel & { children: Channel[] }

    expect(getRoutingTargetCount(aggregate)).toBe(3)
  })
})
