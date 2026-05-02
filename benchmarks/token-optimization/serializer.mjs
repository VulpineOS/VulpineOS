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
const LANDMARK_CODES = new Set(['doc', 'main', 'nav', 'hdr', 'ftr', 'aside', 'form', 'search'])
const STRUCTURAL_CODES = new Set(['sec', 'art', 'ul', 'li', 'tbl', 'row', 'cell'])
const HEADING_RE = /^h[1-6]$/
const HIGH_VALUE_TEXT_RE = /[$£€¥₹]|\b(price|total|stock|available|error|required|warning|sale|deal|shipping|delivery|warranty)\b/i

function textOf(node) {
  return (node.textContent || '').replace(/\s+/g, ' ').trim()
}

function normalizePattern(value) {
  return String(value || '')
    .toLowerCase()
    .replace(/[$£€¥₹]\s*\d+(?:[.,]\d+)?/g, '$#')
    .replace(/\b\d+(?:[.,]\d+)?\b/g, '#')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80)
}

function patternFor(candidate) {
  return `${candidate.code}:${normalizePattern(candidate.name)}`
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

function priorityFor(candidate) {
  const { code, depth, name, props, interactive } = candidate
  let score = 0
  if (code === 'doc') score += 1000
  else if (LANDMARK_CODES.has(code)) score += 850
  else if (code === 'h1') score += 840
  else if (code === 'h2') score += 800
  else if (HEADING_RE.test(code)) score += 620
  else if (interactive) score += 780
  else if (HIGH_VALUE_TEXT_RE.test(name)) score += 790
  else if (code === 't' && name) score += 360
  else if (STRUCTURAL_CODES.has(code)) score += 260
  else score += 180

  if (props) score += 40
  if (name.length > 0 && name.length <= 80) score += 18
  if (name.length > 120) score -= 20
  if (candidate.patternIndex === 0) score += interactive || HEADING_RE.test(code) ? 170 : 120
  else if (candidate.patternIndex < 3) score += 70
  if (candidate.serial < 240) score += 120 - candidate.serial * 0.35
  return score - depth * 8 - candidate.serial * 0.0001
}

function patternLimit(candidate) {
  if (candidate.code === 'doc') return Infinity
  if (LANDMARK_CODES.has(candidate.code)) return 24
  if (HEADING_RE.test(candidate.code)) return 80
  if (candidate.interactive) return 32
  if (HIGH_VALUE_TEXT_RE.test(candidate.name)) return 40
  if (candidate.code === 't') return 18
  return 24
}

function selectCandidates(candidates, maxNodes) {
  if (candidates.length <= maxNodes) return candidates

  const selected = []
  const selectedSet = new Set()
  const patternCounts = new Map()
  const sorted = [...candidates].sort((a, b) => b.score - a.score)

  for (const candidate of sorted) {
    if (selected.length >= maxNodes) break
    const pattern = candidate.pattern
    const count = patternCounts.get(pattern) || 0
    if (count >= patternLimit(candidate)) continue
    selected.push(candidate)
    selectedSet.add(candidate)
    patternCounts.set(pattern, count + 1)
  }

  if (selected.length < maxNodes) {
    for (const candidate of candidates) {
      if (selected.length >= maxNodes) break
      if (selectedSet.has(candidate)) continue
      selected.push(candidate)
      selectedSet.add(candidate)
    }
  }

  return selected.sort((a, b) => a.serial - b.serial)
}

function annotateCandidates(candidates) {
  const patternCounts = new Map()
  for (const candidate of candidates) {
    const pattern = patternFor(candidate)
    const count = patternCounts.get(pattern) || 0
    candidate.pattern = pattern
    candidate.patternIndex = count
    patternCounts.set(pattern, count + 1)
  }
  for (const candidate of candidates)
    candidate.score = priorityFor(candidate)
}

export function getVulpineOptimizedDOM({
  maxDepth = 10,
  maxNodes = 180,
  maxTextLength = 90,
  viewportOnly = false,
} = {}) {
  const candidates = []
  let truncated = false
  let serial = 0
  let refCounter = 0
  const vpWidth = window.innerWidth
  const vpHeight = window.innerHeight
  const scanLimit = Math.max(maxNodes * 10, maxNodes + 500)

  function walkElement(el, depth) {
    if (candidates.length >= scanLimit) {
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
    const interactive = INTERACTIVE_CODES.has(code)
    const candidate = {
      depth,
      code,
      name: ownText,
      props,
      interactive,
      serial: serial++,
    }
    candidates.push(candidate)

    if (NAME_FROM_CONTENT.has(role)) return

    for (const child of childNodes) walkNode(child, depth + 1)
  }

  function walkNode(node, depth) {
    if (candidates.length >= scanLimit) {
      truncated = true
      return
    }
    if (node.nodeType === Node.TEXT_NODE) {
      const value = (node.nodeValue || '').replace(/\s+/g, ' ').trim()
      if (value) {
        const name = value.length > maxTextLength ? value.slice(0, maxTextLength) + '...' : value
        const candidate = {
          depth,
          code: 't',
          name,
          props: null,
          interactive: false,
          serial: serial++,
        }
        candidates.push(candidate)
      }
      return
    }
    walkElement(node, depth)
  }

  walkNode(document.documentElement, 0)

  annotateCandidates(candidates)
  const selected = selectCandidates(candidates, maxNodes)
  truncated = truncated || selected.length < candidates.length

  const nodes = selected.map((candidate) => {
    const tuple = [candidate.depth, candidate.code]
    if (candidate.name || candidate.props || candidate.interactive) tuple.push(candidate.name)
    if (candidate.props || candidate.interactive) tuple.push(candidate.props || null)
    if (candidate.interactive) tuple.push('@' + refCounter++)
    return tuple
  })

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
