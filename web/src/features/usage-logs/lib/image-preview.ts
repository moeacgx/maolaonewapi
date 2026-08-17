/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
export interface ResolvedPreviewImage {
  src: string
  release: () => void
}

export async function resolvePreviewImage(
  source: string,
  signal: AbortSignal,
  loadBlob: (source: string, signal: AbortSignal) => Promise<Blob>
): Promise<ResolvedPreviewImage> {
  if (!source.startsWith('/api/task/')) {
    return { src: source, release: () => undefined }
  }

  const blob = await loadBlob(source, signal)
  if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
  const objectUrl = URL.createObjectURL(blob)
  let released = false
  return {
    src: objectUrl,
    release: () => {
      if (released) return
      released = true
      URL.revokeObjectURL(objectUrl)
    },
  }
}
