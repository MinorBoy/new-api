import { createFileRoute } from '@tanstack/react-router'

import { Main } from '@/components/layout'
import { VideoGeneration } from '@/features/video-generation'

export const Route = createFileRoute('/_authenticated/video-generation/')({
  component: VideoGenerationPage,
})

function VideoGenerationPage() {
  return (
    <Main className='p-0'>
      <VideoGeneration />
    </Main>
  )
}
