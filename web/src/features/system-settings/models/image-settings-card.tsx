import { ImagePricingWorkbench } from './image-pricing-workbench'

type ImageSettingsCardProps = {
  catalog: string
  routing: string
}

export function ImageSettingsCard(props: ImageSettingsCardProps) {
  return (
    <ImagePricingWorkbench catalog={props.catalog} routing={props.routing} />
  )
}
