import config from '../../playwright.config'

describe('Playwright 配置', () => {
  it('在 CI 禁止复用可能残留的本地服务器', () => {
    const webServer = Array.isArray(config.webServer) ? config.webServer[0] : config.webServer

    expect(webServer?.command).toMatch(/^pnpm dev /)
    expect(webServer?.reuseExistingServer).toBe(!process.env.CI)
  })

  it('使用工作流安装的 Playwright Chromium', () => {
    const project = config.projects?.[0]

    expect(project?.use?.channel).toBeUndefined()
  })
})
