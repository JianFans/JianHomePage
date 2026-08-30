export const coverageThresholds = {
  lines: 80,
  statements: 80,
  functions: 75,
  branches: 70,
}

export function coverageConfig(workspace, include, exclude = []) {
  return {
    provider: /** @type {const} */ ('v8'),
    reporter: [/** @type {const} */ ('text'), /** @type {const} */ ('json-summary')],
    reportsDirectory: `../../coverage/${workspace}`,
    include,
    exclude,
    thresholds: coverageThresholds,
  }
}
