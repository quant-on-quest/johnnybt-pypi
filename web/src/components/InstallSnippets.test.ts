import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import InstallSnippets, { buildSnippets } from './InstallSnippets.vue'

const indexUrl = 'https://pypi.example.com/simple/'
const token = 'jbt_abcdefghijklmnopqrstuvwxyz0123456789ABCD'

describe('install snippets', () => {
  it('builds uv, pip and config snippets with the token embedded', () => {
    const s = buildSnippets({ indexUrl, token, packageName: 'johnnybt-demo' })
    expect(s.uvCli).toBe(`uv pip install --index-url https://__token__:${token}@pypi.example.com/simple/ johnnybt-demo`)
    expect(s.pipCli).toBe(`pip install --index-url https://__token__:${token}@pypi.example.com/simple/ johnnybt-demo`)
    expect(s.uvToml).toContain('[[index]]')
    expect(s.uvToml).toContain('name = "johnnybt"')
    expect(s.uvToml).toContain(`url = "${indexUrl}"`)
    expect(s.uvEnv).toContain('UV_INDEX_JOHNNYBT_USERNAME=__token__')
    expect(s.uvEnv).toContain(`UV_INDEX_JOHNNYBT_PASSWORD=${token}`)
    expect(s.pipConf).toContain('[global]')
    expect(s.pipConf).toContain(`extra-index-url = https://__token__:${token}@pypi.example.com/simple/`)
  })

  it('uses a placeholder when no token is known and no package when unspecified', () => {
    const s = buildSnippets({ indexUrl })
    expect(s.uvCli).toBe('uv pip install --index-url https://__token__:<你的token>@pypi.example.com/simple/ <package>')
  })

  it('renders every snippet', () => {
    const w = mount(InstallSnippets, { props: { indexUrl, token, packageName: 'johnnybt-demo' }, global: { stubs: { CopyButton: true } } })
    expect(w.text()).toContain('uv pip install')
    expect(w.text()).toContain('UV_INDEX_JOHNNYBT_PASSWORD')
    expect(w.text()).toContain('extra-index-url')
  })
})
