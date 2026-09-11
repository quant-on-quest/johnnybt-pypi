import { config } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

// Dates are shown in the viewer's zone; pin it so CI (UTC) and laptops (UTC+8)
// agree on what "2026-12-31 到期" means.
process.env.TZ = 'Asia/Shanghai'

/**
 * Nuxt UI components are replaced by transparent stubs that render every slot
 * (named ones too) so page tests can assert on text without booting Reka UI in
 * jsdom. Our own components render for real.
 */
function slotStub(name: string) {
  return defineComponent({
    name,
    inheritAttrs: false,
    props: {
      // Common text-bearing props: render them so assertions see labels/titles.
      label: { type: String, default: undefined },
      title: { type: String, default: undefined },
      description: { type: String, default: undefined },
      items: { type: Array, default: undefined },
      modelValue: { type: null, default: undefined },
      to: { type: null, default: undefined },
    },
    // `click` stays in attrs so it lands on the root element as a DOM listener.
    emits: ['submit', 'update:modelValue'],
    setup(props, { slots, attrs }) {
      return () =>
        h(
          'div',
          { ...attrs, 'data-stub': name },
          [
            props.label,
            props.title,
            props.description,
            typeof props.modelValue === 'string' ? props.modelValue : undefined,
            // USelect/USelectMenu: list option labels so "write" style assertions work.
            ...((props.items as Array<{ label?: string } | string> | undefined)?.map((i) => (typeof i === 'string' ? i : i.label)) ?? []),
            ...Object.keys(slots).map((k) => slots[k]?.()),
          ],
        )
    },
  })
}

const nuxtUi = ['App', 'Form', 'FormField', 'Input', 'Textarea', 'Button', 'Card', 'Badge', 'Alert', 'Icon', 'Select', 'SelectMenu', 'FileUpload', 'Separator']

config.global.stubs = Object.fromEntries(nuxtUi.map((n) => [n, slotStub(n)]))
config.global.renderStubDefaultSlot = true
