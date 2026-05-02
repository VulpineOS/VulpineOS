const ROLE_MAP = {
  document: 'doc',
  main: 'main',
  navigation: 'nav',
  banner: 'hdr',
  contentinfo: 'ftr',
  complementary: 'aside',
  region: 'sec',
  article: 'art',
  form: 'form',
  button: 'btn',
  link: 'a',
  textbox: 'inp',
  searchbox: 'inp',
  checkbox: 'chk',
  radio: 'rad',
  combobox: 'sel',
  listbox: 'sel',
  option: 'opt',
  list: 'ul',
  listitem: 'li',
  table: 'tbl',
  row: 'row',
  cell: 'cell',
  img: 'img',
  text: 't',
}

const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'TEMPLATE', 'NOSCRIPT', 'META', 'LINK'])
const SKIP_ROLES = new Set(['paragraph', 'generic', 'none', 'presentation'])
const NAME_FROM_CONTENT = new Set([
  'heading',
  'button',
  'link',
  'checkbox',
  'radio',
  'option',
  'img',
])
const INTERACTIVE_CODES = new Set(['btn', 'a', 'inp', 'chk', 'rad', 'sel', 'opt'])

function textOf(node) {
  return (node.textContent || '').replace(/\s+/g, ' ').trim()
}

function accessibleName(el, role) {
  const labelledBy = el.getAttribute('aria-labelledby')
  if (labelledBy) {
    const text = labelledBy
      .split(/\s+/)
      .map((id) => el.ownerDocument.getElementById(id)?.textContent || '')
      .join(' ')
      .replace(/\s+/g, ' ')
      .trim()
    if (text) return text
  }
  for (const attr of ['aria-label', 'alt', 'title', 'placeholder']) {
    const value = el.getAttribute(attr)
    if (value && value.trim()) return value.trim()
  }
  if (['textbox', 'searchbox', 'checkbox', 'radio', 'combobox'].includes(role)) {
    const label = el.closest('label')
    if (label) {
      const labelText = textOf(label)
      if (labelText) return labelText
    }
  }
  if (role === 'textbox' || role === 'searchbox') return el.value || ''
  if (!NAME_FROM_CONTENT.has(role)) return ''
  return textOf(el)
}

function roleFor(el) {
  const explicit = el.getAttribute('role')
  if (explicit) return explicit.split(/\s+/)[0]

  const tag = el.tagName
  if (/^H[1-6]$/.test(tag)) return 'heading'
  if (tag === 'A' && el.hasAttribute('href')) return 'link'
  if (tag === 'BUTTON') return 'button'
  if (tag === 'SELECT') return 'combobox'
  if (tag === 'TEXTAREA') return 'textbox'
  if (tag === 'IMG') return 'img'
  if (tag === 'NAV') return 'navigation'
  if (tag === 'MAIN') return 'main'
  if (tag === 'HEADER') return 'banner'
  if (tag === 'FOOTER') return 'contentinfo'
  if (tag === 'ASIDE') return 'complementary'
  if (tag === 'SECTION') return 'region'
  if (tag === 'ARTICLE') return 'article'
  if (tag === 'FORM') return 'form'
  if (tag === 'UL' || tag === 'OL') return 'list'
  if (tag === 'LI') return 'listitem'
  if (tag === 'TABLE') return 'table'
  if (tag === 'TR') return 'row'
  if (tag === 'TD' || tag === 'TH') return 'cell'
  if (tag === 'P') return 'paragraph'
  if (tag === 'INPUT') {
    const type = (el.getAttribute('type') || 'text').toLowerCase()
    if (type === 'checkbox') return 'checkbox'
    if (type === 'radio') return 'radio'
    if (type === 'search') return 'searchbox'
    if (['button', 'submit', 'reset'].includes(type)) return 'button'
    return 'textbox'
  }
  return null
}

function roleCode(el, role) {
  if (role === 'heading') return 'h' + (el.tagName?.slice(1) || el.getAttribute('aria-level') || '1')
  return ROLE_MAP[role] || null
}

function isHidden(el) {
  if (el.getAttribute('aria-hidden') === 'true' || el.hidden) return true
  const style = getComputedStyle(el)
  if (style.display === 'none') return true
  if (style.visibility === 'hidden' || style.visibility === 'collapse') return true
  if (Number(style.opacity) === 0) return true
  if (style.clipPath === 'inset(100%)' || style.clip === 'rect(0px, 0px, 0px, 0px)') return true
  const rect = el.getBoundingClientRect()
  if (rect.width === 0 && rect.height === 0 && style.overflow === 'hidden') return true
  if (rect.bottom < -500 || rect.right < -500) return true
  return false
}

function propsFor(el, role) {
  const props = {}
  if (role === 'link' && el.href) props.hr = el.getAttribute('href') || el.href
  if ((role === 'textbox' || role === 'searchbox') && el.type && el.type !== 'text') props.ty = el.type
  if (el.disabled || el.getAttribute('aria-disabled') === 'true') props.dis = true
  if (el.required || el.getAttribute('aria-required') === 'true') props.req = true
  if (el.checked || el.getAttribute('aria-checked') === 'true') props.ch = true
  if (el.selected || el.getAttribute('aria-selected') === 'true') props.sel = true
  if (el.readOnly) props.ro = true
  if (el.getAttribute('aria-expanded') === 'true') props.exp = true
  if (el.getAttribute('aria-expanded') === 'false') props.exp = false
  if (el.getAttribute('aria-haspopup')) props.pop = el.getAttribute('aria-haspopup')
  return Object.keys(props).length ? props : null
}

export function getVulpineOptimizedDOM({
  maxDepth = 10,
  maxNodes = 250,
  maxTextLength = 120,
  viewportOnly = false,
} = {}) {
  const nodes = []
  let truncated = false
  let refCounter = 0
  const vpWidth = window.innerWidth
  const vpHeight = window.innerHeight

  function walkElement(el, depth) {
    if (nodes.length >= maxNodes) {
      truncated = true
      return
    }
    if (depth > maxDepth) {
      truncated = true
      return
    }
    if (el.nodeType !== Node.ELEMENT_NODE || SKIP_TAGS.has(el.tagName) || isHidden(el)) return

    if (viewportOnly) {
      const rect = el.getBoundingClientRect()
      if (rect.bottom < 0 || rect.top > vpHeight || rect.right < 0 || rect.left > vpWidth) return
    }

    const role = roleFor(el)
    const code = roleCode(el, role)
    const childNodes = Array.from(el.childNodes)

    if (!code || SKIP_ROLES.has(role)) {
      for (const child of childNodes) walkNode(child, depth)
      return
    }

    const name = accessibleName(el, role)
    const ownText = name.length > maxTextLength ? name.slice(0, maxTextLength) + '...' : name
    const props = propsFor(el, role)
    const ref = INTERACTIVE_CODES.has(code) ? '@' + refCounter++ : null

    if (ref) nodes.push([depth, code, ownText, props || null, ref])
    else if (props) nodes.push([depth, code, ownText, props])
    else if (ownText) nodes.push([depth, code, ownText])
    else nodes.push([depth, code])

    if (NAME_FROM_CONTENT.has(role)) return

    for (const child of childNodes) walkNode(child, depth + 1)
  }

  function walkNode(node, depth) {
    if (nodes.length >= maxNodes) {
      truncated = true
      return
    }
    if (node.nodeType === Node.TEXT_NODE) {
      const value = (node.nodeValue || '').replace(/\s+/g, ' ').trim()
      if (value) nodes.push([depth, 't', value.length > maxTextLength ? value.slice(0, maxTextLength) + '...' : value])
      return
    }
    walkElement(node, depth)
  }

  walkNode(document.documentElement, 0)

  const merged = []
  for (const node of nodes) {
    const previous = merged[merged.length - 1]
    if (node[1] === 't' && previous?.[1] === 't' && previous[0] === node[0] && !previous[3] && !node[3]) {
      previous[2] = `${previous[2] || ''} ${node[2] || ''}`.trim()
    } else {
      merged.push(node)
    }
  }

  return {
    snapshot: {
      v: 1,
      title: document.title || '',
      url: window.location.href || '',
      nodes: merged,
    },
    truncated,
  }
}
