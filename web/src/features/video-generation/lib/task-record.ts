import type { VideoTaskRecord } from '../types'

export function appendTaskRecord(
  records: VideoTaskRecord[],
  record: VideoTaskRecord
): VideoTaskRecord[] {
  return [...records, record]
}

export function updateTaskRecord(
  records: VideoTaskRecord[],
  clientId: string,
  patch: Partial<VideoTaskRecord>
): VideoTaskRecord[] {
  return records.map((record) =>
    record.clientId === clientId ? { ...record, ...patch } : record
  )
}
