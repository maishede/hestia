import { number, t } from './i18n.js'
import { esc } from './util.js'

export function actorFilterMarkup() {
  return `
    <aside class="actor-sidebar" aria-label="${esc(t('actorFilter'))}" hidden>
      <h2>${esc(t('actors'))}</h2>
      <search class="actor-search">
        <input id="actorSearch" type="search" aria-label="${esc(t('actorSearch'))}" aria-controls="actorList" placeholder="${esc(t('actorSearchHint'))}" autocomplete="off" spellcheck="false">
      </search>
      <div id="actorList" class="actor-list"></div>
      <div id="actorEmpty" class="actor-empty" hidden></div>
    </aside>
    <div class="actor-picker" hidden>
      <label for="actorPicker">${esc(t('actors'))}</label>
      <select class="sort-sel" id="actorPicker"></select>
    </div>`
}

export function bindActorFilter(root, { onSelect, allLabel, alwaysVisible = false, initial = {} }) {
  const list = root.querySelector('#actorList')
  const input = root.querySelector('#actorSearch')
  const empty = root.querySelector('#actorEmpty')
  const picker = root.querySelector('#actorPicker')
  input.value = initial.actorSearch ?? ''
  let choices = []
  let scopeEmpty = ''
  let initialScroll = initial.actorScroll ?? 0

  function filter() {
    const query = input.value.trim().toLowerCase()
    let matches = 0
    for (const button of list.children) {
      const name = button.dataset.actor
      button.hidden = !!name && !name.toLowerCase().includes(query)
      if (name && !button.hidden) matches++
    }
    empty.textContent = scopeEmpty || (query && matches === 0 ? t('actorEmpty') : '')
    empty.hidden = !empty.textContent
  }

  function select(name) {
    list.querySelector('[aria-pressed="true"]')?.setAttribute('aria-pressed', 'false')
    list.querySelector(`[data-actor="${CSS.escape(name)}"]`)?.setAttribute('aria-pressed', 'true')
    picker.value = name
  }

  function choose(name) {
    select(name)
    onSelect(name)
  }

  input.addEventListener('input', filter)
  picker.onchange = () => choose(picker.value)
  list.onclick = event => {
    const button = event.target.closest('button[data-actor]')
    if (button) choose(button.dataset.actor)
  }

  return {
    render(actors, selected, emptyMessage = '') {
      const visible = alwaysVisible || actors.length > 0 || !!selected
      list.parentElement.hidden = picker.parentElement.hidden = !visible
      root.classList.toggle('with-actors', visible)
      const next = [{ name: '', label: allLabel }, ...actors.map(({ name, count }) => ({ name, count, label: name }))]
      if (selected && !actors.some(a => a.name === selected)) next.push({ name: selected, label: selected, count: 0 })
      if (next.length !== choices.length || next.some(({ name, count }, i) => name !== choices[i].name || count !== choices[i].count)) {
        const scroll = initialScroll || list.scrollTop
        choices = next
        list.innerHTML = choices.map(({ name, label, count }) =>
          `<button type="button" data-actor="${esc(name)}" aria-pressed="false">
            <span>${esc(label)}</span>${count == null ? '' : `<small>${number(count)}</small>`}
          </button>`).join('')
        picker.replaceChildren(...choices.map(({ name, label }) => new Option(label, name)))
        scopeEmpty = emptyMessage
        filter()
        list.scrollTop = scroll
        if (actors.length) initialScroll = 0
      } else if (scopeEmpty !== emptyMessage) {
        scopeEmpty = emptyMessage
        filter()
      }
      select(selected)
    },
    select,
    saveState() { return { actorSearch: input.value, actorScroll: list.scrollTop } },
  }
}
