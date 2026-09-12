export function countUnsavedImagePricingChanges(
  edited: Record<string, string>,
  saved: Record<string, string>
): number {
  return Object.keys(edited).filter((id) => edited[id] !== saved[id]).length
}
