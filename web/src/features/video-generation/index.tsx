import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import axios from 'axios'
import {
  Braces,
  Copy,
  Film,
  KeyRound,
  Plus,
  RotateCcw,
  Send,
  Sparkles,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import type { ApiKey } from '../keys/types'
import {
  createVideoTask,
  getVideoApiKeys,
  getVideoApiKeyValue,
  getVideoModels,
  queryVideoTask,
} from './api'
import { ReferenceMediaEditor } from './components/reference-media-editor'
import { VideoTaskCard } from './components/video-task-card'
import { DEFAULT_VIDEO_FORM } from './lib/defaults'
import {
  isTerminalVideoTaskStatus,
  shouldScheduleNextVideoPoll,
} from './lib/polling'
import {
  buildVideoRequest,
  buildVideoTaskCurl,
  filterModelsForApiKey,
} from './lib/request'
import { createVideoGenerationSchema } from './lib/schema'
import { appendTaskRecord, updateTaskRecord } from './lib/task-record'
import { getVideoTaskId, getVideoTaskStatus } from './lib/task-state'
import type {
  VideoGenerationForm,
  VideoTaskRecord,
  VideoTaskStatus,
} from './types'

const POLL_INTERVAL_MS = 5000

function cloneDefaultForm(): VideoGenerationForm {
  return {
    ...DEFAULT_VIDEO_FORM,
    media: {
      images: [...DEFAULT_VIDEO_FORM.media.images],
      videos: [...DEFAULT_VIDEO_FORM.media.videos],
      audios: [...DEFAULT_VIDEO_FORM.media.audios],
    },
  }
}

function getErrorMessage(error: unknown): string {
  if (axios.isAxiosError(error)) {
    const body = error.response?.data as
      | { error?: { message?: string }; message?: string }
      | undefined
    return body?.error?.message || body?.message || error.message
  }
  return error instanceof Error ? error.message : 'Request failed'
}

async function copyText(value: string): Promise<void> {
  await navigator.clipboard.writeText(value)
}

function selectedKeyLabel(apiKey: ApiKey): string {
  const group = apiKey.group || 'default'
  return `${apiKey.name} · ${group} · ${apiKey.key}`
}

export function VideoGeneration() {
  const { t } = useTranslation()
  const [selectedKeyId, setSelectedKeyId] = useState('')
  const [tasks, setTasks] = useState<VideoTaskRecord[]>([])
  const [isCreating, setIsCreating] = useState(false)
  const isMounted = useRef(false)
  const pollTimers = useRef(new Map<string, number>())
  const pollRuns = useRef(new Map<string, number>())
  const schema = useMemo(() => createVideoGenerationSchema(t), [t])
  const form = useForm<VideoGenerationForm>({
    resolver: zodResolver(schema),
    defaultValues: cloneDefaultForm(),
  })

  const apiKeysQuery = useQuery({
    queryKey: ['video-generation', 'api-keys'],
    queryFn: getVideoApiKeys,
    staleTime: 30_000,
  })
  const apiKeys = apiKeysQuery.data ?? []
  const selectedKey = apiKeys.find(
    (apiKey) => String(apiKey.id) === selectedKeyId
  )
  const selectedGroup = selectedKey?.group || 'default'
  const modelsQuery = useQuery({
    queryKey: ['video-generation', 'models', selectedGroup],
    queryFn: () => getVideoModels(selectedGroup),
    enabled: Boolean(selectedKey),
    staleTime: 30_000,
  })
  const models = useMemo(
    () =>
      selectedKey
        ? filterModelsForApiKey(modelsQuery.data ?? [], selectedKey)
        : [],
    [modelsQuery.data, selectedKey]
  )
  const media = form.watch('media')
  const prompt = form.watch('prompt')
  const requestPreview = buildVideoRequest(form.watch())
  let modelPlaceholder = t('No models available')
  if (modelsQuery.isLoading) {
    modelPlaceholder = t('Loading models...')
  } else if (models.length > 0) {
    modelPlaceholder = t('Select a model')
  }

  useEffect(() => {
    if (!selectedKey) {
      form.setValue('model', '')
      return
    }
    const currentModel = form.getValues('model')
    if (models.includes(currentModel)) return
    const defaultModel = models.includes(DEFAULT_VIDEO_FORM.model)
      ? DEFAULT_VIDEO_FORM.model
      : models[0] || ''
    form.setValue('model', defaultModel, { shouldValidate: true })
  }, [form, models, selectedKey])

  useEffect(() => {
    isMounted.current = true
    const timers = pollTimers.current
    const runs = pollRuns.current

    return () => {
      isMounted.current = false
      timers.forEach((timer) => window.clearTimeout(timer))
      timers.clear()
      runs.clear()
    }
  }, [])

  const patchTask = useCallback(
    (clientId: string, patch: Partial<VideoTaskRecord>) => {
      setTasks((current) => updateTaskRecord(current, clientId, patch))
    },
    []
  )

  const pollTask = useCallback(
    async (
      clientId: string,
      taskId: string,
      apiKey: string,
      pollRun: number
    ) => {
      const isCurrent = () =>
        isMounted.current && pollRuns.current.get(clientId) === pollRun
      if (!isCurrent()) return

      let status: VideoTaskStatus = 'unknown'
      let requestFailed = false
      try {
        const response = await queryVideoTask(taskId, apiKey)
        if (!isCurrent()) return
        status = getVideoTaskStatus(response)
        patchTask(clientId, { response, status, error: undefined })
      } catch (error) {
        if (!isCurrent()) return
        requestFailed = true
        patchTask(clientId, { error: getErrorMessage(error) })
      }

      if (
        !shouldScheduleNextVideoPoll({
          isCurrent: isCurrent(),
          status,
          requestFailed,
        })
      ) {
        pollTimers.current.delete(clientId)
        return
      }

      const timer = window.setTimeout(
        () => void pollTask(clientId, taskId, apiKey, pollRun),
        POLL_INTERVAL_MS
      )
      pollTimers.current.set(clientId, timer)
    },
    [patchTask]
  )

  const startPolling = useCallback(
    (clientId: string, taskId: string, apiKey: string) => {
      const existingTimer = pollTimers.current.get(clientId)
      if (existingTimer) window.clearTimeout(existingTimer)
      pollTimers.current.delete(clientId)
      const pollRun = (pollRuns.current.get(clientId) ?? 0) + 1
      pollRuns.current.set(clientId, pollRun)
      void pollTask(clientId, taskId, apiKey, pollRun)
    },
    [pollTask]
  )

  async function submitTask(values: VideoGenerationForm) {
    if (!selectedKey) return
    setIsCreating(true)
    const request = buildVideoRequest(values)
    const clientId = crypto.randomUUID()
    const record: VideoTaskRecord = {
      clientId,
      taskId: '',
      status: 'queued',
      request,
      createdAt: Date.now(),
      apiKey: '',
    }
    setTasks((current) => appendTaskRecord(current, record))

    try {
      const apiKey = await getVideoApiKeyValue(selectedKey.id)
      const response = await createVideoTask(request, apiKey)
      const taskId = getVideoTaskId(response)
      if (!taskId) throw new Error(t('The server did not return a task ID'))
      const status = getVideoTaskStatus(response)
      patchTask(clientId, { apiKey, response, status, taskId })
      toast.success(t('Video task created'))
      if (!isTerminalVideoTaskStatus(status)) {
        startPolling(clientId, taskId, apiKey)
      }
    } catch (error) {
      patchTask(clientId, { status: 'failed', error: getErrorMessage(error) })
      toast.error(getErrorMessage(error))
    } finally {
      setIsCreating(false)
    }
  }

  async function refreshTask(task: VideoTaskRecord) {
    if (!task.taskId || !task.apiKey) return
    startPolling(task.clientId, task.taskId, task.apiKey)
  }

  async function copy(value: string, successMessage: string) {
    try {
      await copyText(value)
      toast.success(t(successMessage))
    } catch {
      toast.error(t('Clipboard access failed'))
    }
  }

  function resetForm() {
    const defaults = cloneDefaultForm()
    if (selectedKey && !models.includes(defaults.model)) {
      defaults.model = models[0] || ''
    }
    form.reset(defaults)
  }

  return (
    <div className='flex h-full min-h-0 flex-col overflow-y-auto'>
      <div className='mx-auto flex w-full max-w-[1600px] flex-col gap-5 p-4 sm:p-6'>
        <header className='flex flex-col justify-between gap-3 border-b pb-5 lg:flex-row lg:items-end'>
          <div>
            <div className='text-primary mb-2 flex items-center gap-2 text-xs font-medium'>
              <Sparkles className='size-4' aria-hidden='true' />
              Seedance 2.0
            </div>
            <h1 className='text-2xl font-semibold'>{t('Video generation')}</h1>
            <p className='text-muted-foreground mt-1 max-w-3xl text-sm'>
              {t(
                'Create and monitor multimodal video tasks with your existing API keys.'
              )}
            </p>
          </div>
          <div className='flex flex-wrap gap-2'>
            <Badge variant='outline'>{t('Images')} 0–9</Badge>
            <Badge variant='outline'>{t('Videos')} 0–3</Badge>
            <Badge variant='outline'>{t('Audio')} 0–3</Badge>
          </div>
        </header>

        <Card>
          <CardContent className='grid gap-4 pt-0 lg:grid-cols-2'>
            <div className='space-y-2'>
              <FieldLabel htmlFor='video-api-key'>
                <KeyRound className='size-4' aria-hidden='true' />
                {t('API key')}
              </FieldLabel>
              <NativeSelect
                id='video-api-key'
                aria-label={t('API key')}
                className='w-full'
                disabled={apiKeysQuery.isLoading}
                value={selectedKeyId}
                onChange={(event) => setSelectedKeyId(event.target.value)}
              >
                <NativeSelectOption value=''>
                  {apiKeysQuery.isLoading
                    ? t('Loading API keys...')
                    : t('Select an API key')}
                </NativeSelectOption>
                {apiKeys.map((apiKey) => (
                  <NativeSelectOption key={apiKey.id} value={String(apiKey.id)}>
                    {selectedKeyLabel(apiKey)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <p className='text-muted-foreground text-xs'>
                {selectedKey
                  ? t('Models are loaded from the {{group}} group.', {
                      group: selectedGroup,
                    })
                  : t(
                      'Choose an enabled API key to load models for its group.'
                    )}
              </p>
            </div>

            <div className='space-y-2'>
              <FieldLabel htmlFor='video-model'>{t('Model')}</FieldLabel>
              <NativeSelect
                id='video-model'
                aria-label={t('Model')}
                className='w-full'
                disabled={
                  !selectedKey || modelsQuery.isLoading || models.length === 0
                }
                {...form.register('model')}
              >
                <NativeSelectOption value=''>
                  {modelPlaceholder}
                </NativeSelectOption>
                {models.map((model) => (
                  <NativeSelectOption key={model} value={model}>
                    {model}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
              <FieldError>{form.formState.errors.model?.message}</FieldError>
            </div>
          </CardContent>
        </Card>

        <form
          className='grid min-w-0 items-start gap-5 xl:grid-cols-[minmax(0,1.45fr)_minmax(360px,0.55fr)]'
          onSubmit={form.handleSubmit(submitTask)}
        >
          <div className='min-w-0 space-y-5'>
            <Card>
              <CardHeader className='border-b pb-4'>
                <CardTitle>{t('Create video task')}</CardTitle>
                <CardDescription>
                  {t(
                    'Prompt and reference URLs are combined into the ARK content array.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-5'>
                <div className='space-y-2'>
                  <div className='flex items-center justify-between gap-3'>
                    <FieldLabel htmlFor='video-prompt'>
                      {t('Prompt')}
                    </FieldLabel>
                    <span className='text-muted-foreground text-xs tabular-nums'>
                      {prompt.length} / 4000
                    </span>
                  </div>
                  <Textarea
                    id='video-prompt'
                    className='min-h-44 resize-y leading-relaxed'
                    maxLength={4000}
                    aria-invalid={Boolean(form.formState.errors.prompt)}
                    {...form.register('prompt')}
                  />
                  <FieldError>
                    {form.formState.errors.prompt?.message}
                  </FieldError>
                </div>

                <div>
                  <div className='mb-3'>
                    <h2 className='text-sm font-medium'>
                      {t('Reference media URLs')}
                    </h2>
                    <p className='text-muted-foreground mt-1 text-xs'>
                      {t('Only public HTTP(S) URLs are supported.')}
                    </p>
                  </div>
                  <div className='grid gap-3 lg:grid-cols-3'>
                    <ReferenceMediaEditor
                      kind='images'
                      values={media.images}
                      onChange={(values) =>
                        form.setValue('media.images', values, {
                          shouldValidate: true,
                        })
                      }
                    />
                    <ReferenceMediaEditor
                      kind='videos'
                      values={media.videos}
                      onChange={(values) =>
                        form.setValue('media.videos', values, {
                          shouldValidate: true,
                        })
                      }
                    />
                    <ReferenceMediaEditor
                      kind='audios'
                      values={media.audios}
                      onChange={(values) =>
                        form.setValue('media.audios', values, {
                          shouldValidate: true,
                        })
                      }
                    />
                  </div>
                  <FieldError className='mt-2'>
                    {form.formState.errors.media?.images?.message ||
                      form.formState.errors.media?.videos?.message ||
                      form.formState.errors.media?.audios?.message}
                  </FieldError>
                </div>

                <details className='rounded-lg border'>
                  <summary className='cursor-pointer px-4 py-3 text-sm font-medium'>
                    {t('Generation options')}
                  </summary>
                  <div className='grid gap-4 border-t p-4 sm:grid-cols-2 lg:grid-cols-3'>
                    <div className='space-y-2'>
                      <FieldLabel htmlFor='video-resolution'>
                        {t('Resolution')}
                      </FieldLabel>
                      <NativeSelect
                        id='video-resolution'
                        className='w-full'
                        {...form.register('resolution')}
                      >
                        <NativeSelectOption value=''>
                          {t('Default')}
                        </NativeSelectOption>
                        {['480p', '720p', '1080p', '4k'].map((resolution) => (
                          <NativeSelectOption
                            key={resolution}
                            value={resolution}
                          >
                            {resolution === '4k' ? '4K' : resolution}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                    </div>
                    <div className='space-y-2'>
                      <FieldLabel htmlFor='video-ratio'>
                        {t('Aspect ratio')}
                      </FieldLabel>
                      <NativeSelect
                        id='video-ratio'
                        className='w-full'
                        {...form.register('ratio')}
                      >
                        {[
                          'adaptive',
                          '16:9',
                          '4:3',
                          '1:1',
                          '3:4',
                          '9:16',
                          '21:9',
                        ].map((ratio) => (
                          <NativeSelectOption key={ratio} value={ratio}>
                            {ratio}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                    </div>
                    <div className='space-y-2'>
                      <FieldLabel htmlFor='video-duration'>
                        {t('Duration (seconds)')}
                      </FieldLabel>
                      <Input
                        id='video-duration'
                        type='number'
                        min={-1}
                        max={15}
                        aria-invalid={Boolean(form.formState.errors.duration)}
                        {...form.register('duration', { valueAsNumber: true })}
                      />
                      <FieldError>
                        {form.formState.errors.duration?.message}
                      </FieldError>
                    </div>
                    <div className='space-y-2'>
                      <FieldLabel htmlFor='video-expiry'>
                        {t('Task expiry (seconds)')}
                      </FieldLabel>
                      <Input
                        id='video-expiry'
                        type='number'
                        min={3600}
                        max={259200}
                        aria-invalid={Boolean(
                          form.formState.errors.executionExpiresAfter
                        )}
                        {...form.register('executionExpiresAfter', {
                          valueAsNumber: true,
                        })}
                      />
                      <FieldError>
                        {form.formState.errors.executionExpiresAfter?.message}
                      </FieldError>
                    </div>
                    <div className='space-y-2 sm:col-span-2'>
                      <FieldLabel htmlFor='video-callback-url'>
                        {t('Callback URL')}
                      </FieldLabel>
                      <Input
                        id='video-callback-url'
                        inputMode='url'
                        placeholder='https://example.com/webhook'
                        aria-invalid={Boolean(
                          form.formState.errors.callbackUrl
                        )}
                        {...form.register('callbackUrl')}
                      />
                      <FieldError>
                        {form.formState.errors.callbackUrl?.message}
                      </FieldError>
                    </div>
                    <ToggleField
                      checked={form.watch('generateAudio')}
                      label={t('Generate audio')}
                      onCheckedChange={(checked) =>
                        form.setValue('generateAudio', checked)
                      }
                    />
                    <ToggleField
                      checked={form.watch('watermark')}
                      label={t('Add watermark')}
                      onCheckedChange={(checked) =>
                        form.setValue('watermark', checked)
                      }
                    />
                    <ToggleField
                      checked={form.watch('returnLastFrame')}
                      label={t('Return last frame')}
                      onCheckedChange={(checked) =>
                        form.setValue('returnLastFrame', checked)
                      }
                    />
                  </div>
                </details>
              </CardContent>
            </Card>

            <div className='flex flex-col-reverse justify-between gap-3 sm:flex-row sm:items-center'>
              <Button type='button' variant='ghost' onClick={resetForm}>
                <RotateCcw aria-hidden='true' />
                {t('Reset sample')}
              </Button>
              <Button
                className='min-w-40'
                disabled={!selectedKey || models.length === 0 || isCreating}
                type='submit'
              >
                {isCreating ? <Spinner /> : <Send aria-hidden='true' />}
                {isCreating ? t('Creating task...') : t('Create video task')}
              </Button>
            </div>
          </div>

          <Card className='min-w-0 xl:sticky xl:top-4'>
            <CardHeader className='border-b pb-4'>
              <CardTitle className='flex items-center gap-2'>
                <Braces className='size-4' aria-hidden='true' />
                {t('Request JSON')}
              </CardTitle>
              <CardDescription>
                {t('Preview of the payload sent for the next task.')}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-3'>
              <pre className='bg-muted/40 max-h-[560px] overflow-auto rounded-lg border p-3 text-[11px] leading-relaxed break-all whitespace-pre-wrap'>
                {JSON.stringify(requestPreview, null, 2)}
              </pre>
              <div className='flex flex-wrap gap-2'>
                <Button
                  size='sm'
                  type='button'
                  variant='outline'
                  onClick={() =>
                    void copy(
                      JSON.stringify(requestPreview, null, 2),
                      'Request JSON copied'
                    )
                  }
                >
                  <Copy aria-hidden='true' />
                  {t('Copy JSON')}
                </Button>
                <Button
                  size='sm'
                  type='button'
                  variant='outline'
                  onClick={() =>
                    void copy(
                      buildVideoTaskCurl(requestPreview, window.location.origin),
                      'cURL copied'
                    )
                  }
                >
                  <Copy aria-hidden='true' />
                  {t('Copy cURL')}
                </Button>
              </div>
            </CardContent>
          </Card>
        </form>

        <section aria-labelledby='video-task-list-title'>
          <div className='mb-3 flex flex-wrap items-center justify-between gap-3'>
            <div>
              <h2 id='video-task-list-title' className='text-lg font-semibold'>
                {t('Video tasks')}
              </h2>
              <p className='text-muted-foreground text-sm'>
                {t('Each submitted task is monitored independently.')}
              </p>
            </div>
            <Badge variant='secondary'>
              <Plus aria-hidden='true' />
              {t('{{count}} tasks', { count: tasks.length })}
            </Badge>
          </div>
          {tasks.length === 0 ? (
            <div className='text-muted-foreground flex min-h-32 items-center justify-center rounded-lg border border-dashed text-sm'>
              <div className='text-center'>
                <Film className='mx-auto mb-2 size-5' aria-hidden='true' />
                {t('No video tasks yet')}
              </div>
            </div>
          ) : (
            <div className='grid gap-4'>
              {[...tasks].reverse().map((task) => (
                <VideoTaskCard
                  key={task.clientId}
                  task={task}
                  onRefresh={() => void refreshTask(task)}
                  onCopy={(value) => void copy(value, 'Video URL copied')}
                />
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}

type ToggleFieldProps = {
  label: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}

function ToggleField(props: ToggleFieldProps) {
  return (
    <label className='flex min-h-9 items-center justify-between gap-3 rounded-lg border px-3 py-2 text-sm'>
      <span>{props.label}</span>
      <Switch checked={props.checked} onCheckedChange={props.onCheckedChange} />
    </label>
  )
}
