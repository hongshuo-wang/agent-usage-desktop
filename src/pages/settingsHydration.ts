export function applyOwnedHydration<T>(
  mounted: boolean,
  requestGeneration: number,
  currentGeneration: number,
  value: T,
  apply: (value: T) => void,
) {
  if (mounted && requestGeneration === currentGeneration) apply(value);
}
