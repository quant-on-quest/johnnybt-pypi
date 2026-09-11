import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import InstallSnippets, { buildSnippets } from './InstallSnippets.vue'

const indexUrl = 'https://pypi.example.com/simple/'
const token = 'jbt_abcdefghijklmnopqrstuvwxyz0123456789ABCD'

describe('install snippets (uv first)', () => {
  const s = buildSnippets({ indexUrl, token, packageName: 'johnnybt-demo' })

  it('step 1: uv auth login with the token, once', () => {
    expect(s.uvLogin).toBe(`uv auth login ${indexUrl} --token ${token}`)
  })

  it('step 2: global index config that sends credentials eagerly', () => {
    expect(s.uvConfig).toContain('[[index]]')
    expect(s.uvConfig).toContain('name = "johnnybt"')
    expect(s.uvConfig).toContain(`url = "${indexUrl}"`)
    expect(s.uvConfig).toContain('authenticate = "always"')
    expect(s.uvConfig).toContain('~/.config/uv/uv.toml')
    expect(s.uvConfig).toContain('%APPDATA%\\uv\\uv.toml')
  })

  it('step 3: plain uv commands, no credentials anywhere', () => {
    expect(s.uvUse).toContain('uv add johnnybt-demo')
    expect(s.uvUse).toContain('uvx johnnybt-demo')
    expect(s.uvUse).not.toContain(token)
    expect(s.uvUse).not.toContain('__token__')
  })

  it('advanced: project-level pin with explicit index and sources', () => {
    expect(s.projectPin).toContain('[[tool.uv.index]]')
    expect(s.projectPin).toContain('explicit = true')
    expect(s.projectPin).toContain('[tool.uv.sources]')
    expect(s.projectPin).toContain('johnnybt-demo = { index = "johnnybt" }')
  })

  it('advanced: CI env vars and pip fallback still carry the token', () => {
    expect(s.ciEnv).toContain('UV_INDEX_JOHNNYBT_USERNAME=__token__')
    expect(s.ciEnv).toContain(`UV_INDEX_JOHNNYBT_PASSWORD=${token}`)
    expect(s.pipConf).toContain('[global]')
    expect(s.pipConf).toContain(`extra-index-url = https://__token__:${token}@pypi.example.com/simple/`)
    expect(s.pipCli).toBe(`pip install --index-url https://__token__:${token}@pypi.example.com/simple/ johnnybt-demo`)
  })

  it('uses placeholders when token or package are unknown', () => {
    const p = buildSnippets({ indexUrl })
    expect(p.uvLogin).toBe(`uv auth login ${indexUrl} --token <你的token>`)
    expect(p.uvUse).toContain('uv add <package>')
    expect(p.projectPin).toContain('<package> = { index = "johnnybt" }')
  })

  it('renders the uv steps first and the rest under an advanced fold', () => {
    const w = mount(InstallSnippets, { props: { indexUrl, token, packageName: 'johnnybt-demo' }, global: { stubs: { CopyButton: true } } })
    const text = w.text()
    expect(text.indexOf('uv auth login')).toBeLessThan(text.indexOf('authenticate = "always"'))
    expect(text.indexOf('authenticate = "always"')).toBeLessThan(text.indexOf('uv add johnnybt-demo'))
    expect(w.find('details').exists()).toBe(true)
    expect(w.find('details').text()).toContain('UV_INDEX_JOHNNYBT_PASSWORD')
    expect(w.find('details').text()).toContain('extra-index-url')
    expect(text).toContain('uv self update')
  })
})
