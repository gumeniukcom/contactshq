import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import type { Contact, PotentialDuplicate, ValueCandidate } from '@/types'

const getDuplicate = vi.fn()
const mergeContacts = vi.fn()
const listDuplicates = vi.fn()
const push = vi.fn()
const toastSuccess = vi.fn()

vi.mock('@/api/contacts', () => ({
  getDuplicate: (...args: unknown[]) => getDuplicate(...args),
  mergeContacts: (...args: unknown[]) => mergeContacts(...args),
  listDuplicates: (...args: unknown[]) => listDuplicates(...args),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('vue-router')
  return {
    ...actual,
    useRoute: () => ({ params: { dupId: 'd1' } }),
    useRouter: () => ({ push }),
    RouterLink: { template: '<a><slot /></a>' },
  }
})

vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ success: toastSuccess, error: vi.fn(), info: vi.fn() }),
}))

import ContactMergeView from './ContactMergeView.vue'

function contact(id: string, first: string, email: string): Contact {
  return {
    id,
    address_book_id: 'ab1',
    uid: id,
    first_name: first,
    last_name: 'Lovelace',
    email,
    created_at: '2026-01-01T00:00:00Z',
  } as Contact
}

const candidates: ValueCandidate[] = [
  { id: 'fn-a', property: 'FN', value: 'Ada Lovelace', side: 'winner' },
  { id: 'fn-b', property: 'FN', value: 'Ada L', side: 'loser' },
  { id: 'em-a', property: 'EMAIL', value: 'ada@example.com', side: 'winner' },
  { id: 'em-b', property: 'EMAIL', value: 'home@example.com', side: 'loser' },
]

const duplicate: PotentialDuplicate = {
  id: 'd1',
  user_id: 'u1',
  contact_a_id: 'a',
  contact_b_id: 'b',
  score: 1,
  match_reasons: '[{"code":"email_match","value":"ada@example.com"}]',
  status: 'pending',
  created_at: '2026-01-01T00:00:00Z',
  contact_a: contact('a', 'Ada', 'ada@example.com'),
  contact_b: contact('b', 'Ada', 'home@example.com'),
}

function mountView() {
  // attachTo so the confirmation dialog, which AppModal teleports to <body>, is reachable.
  return mount(ContactMergeView, { attachTo: document.body })
}

/** ConfirmDialog lives outside the component tree; drive it through the document. */
async function clickInDialog(text: string) {
  const button = Array.from(document.body.querySelectorAll('button')).find(
    (b) => b.textContent?.trim() === text,
  )
  if (!button) throw new Error(`no "${text}" button in the dialog`)
  button.click()
  await flushPromises()
}

beforeEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
  getDuplicate.mockResolvedValue({ data: { duplicate, candidates } })
  mergeContacts.mockResolvedValue({ data: {} })
})

describe('ContactMergeView', () => {
  // The old view asked for a page of up to 200 pairs and searched it, so a pair outside that
  // page could not be opened at all.
  it('loads the pair by id and never lists', async () => {
    mountView()
    await flushPromises()

    expect(getDuplicate).toHaveBeenCalledTimes(1)
    expect(getDuplicate).toHaveBeenCalledWith('d1')
    expect(listDuplicates).not.toHaveBeenCalled()
  })

  it('renders a group per comparable property', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Display name')
    expect(wrapper.text()).toContain('Emails')
    expect(wrapper.text()).toContain('Which record survives')
  })

  // Multi-valued properties keep everything; a single-valued one can only keep one, so the
  // other display name is discarded by definition — and the screen has to say so.
  it('keeps every multi-value and reports the unavoidable singleton loss', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Will be discarded (1)')
    expect(wrapper.text()).toContain('Ada L')
    // Both emails survive.
    expect(wrapper.text()).toContain('ada@example.com')
    expect(wrapper.text()).toContain('home@example.com')
  })

  // Unticking a value must move it into the discard list immediately, without a request.
  it('moves a deselected value into the discard list', async () => {
    const wrapper = mountView()
    await flushPromises()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)
    await checkboxes[1].setValue(false)

    // The discarded display name, plus the email just unticked.
    expect(wrapper.text()).toContain('Will be discarded (2)')
    expect(wrapper.text()).toContain('home@example.com')
  })

  it('shows a not-found card without a merge button when the pair is gone', async () => {
    getDuplicate.mockRejectedValue({ response: { status: 404 } })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('This pair no longer exists')
    // The heading always says "Merge contacts"; what must be gone is the action.
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Merge contacts')
  })

  it('treats a forbidden pair the same way — there is nothing to retry', async () => {
    getDuplicate.mockRejectedValue({ response: { status: 403 } })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('This pair no longer exists')
  })

  it('offers a retry after a transport failure and repeats the request', async () => {
    getDuplicate.mockRejectedValueOnce(new Error('network down'))

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('Could not load this pair')

    getDuplicate.mockResolvedValue({ data: { duplicate, candidates } })
    const retry = wrapper.findAll('button').find((b) => b.text().includes('Retry'))
    await retry?.trigger('click')
    await flushPromises()

    expect(getDuplicate).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Which record survives')
  })

  it('sends the payload built from the explicit winner and the selection', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Merge contacts')
      ?.trigger('click')
    await flushPromises()
    await clickInDialog('Merge')

    expect(mergeContacts).toHaveBeenCalledTimes(1)
    const payload = mergeContacts.mock.calls[0][0]
    expect(payload.winner_id).toBe('a')
    expect(payload.loser_id).toBe('b')
    expect(payload.dup_id).toBe('d1')
    expect(Object.keys(payload.selection)).toContain('EMAIL')
    expect(push).toHaveBeenCalledWith('/contacts/duplicates')
  })

  // Navigating away on failure would make the user rebuild every choice to try again.
  it('stays mounted and shows the error when the merge fails', async () => {
    mergeContacts.mockRejectedValue({ response: { data: { error: 'merge failed' } } })

    const wrapper = mountView()
    await flushPromises()

    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Merge contacts')
      ?.trigger('click')
    await flushPromises()
    await clickInDialog('Merge')

    expect(push).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('merge failed')
    expect(wrapper.text()).toContain('Which record survives')
  })

  // Choosing the other record changes which single-valued defaults apply.
  it('rebuilds the defaults when the surviving record changes', async () => {
    const wrapper = mountView()
    await flushPromises()

    const winnerRadios = wrapper.findAll('input[name="merge-winner"]')
    expect(winnerRadios).toHaveLength(2)
    await winnerRadios[1].trigger('change')
    await flushPromises()

    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Merge contacts')
      ?.trigger('click')
    await flushPromises()
    await clickInDialog('Merge')

    const payload = mergeContacts.mock.calls[0][0]
    expect(payload.winner_id).toBe('b')
    expect(payload.selection.FN).toEqual(['fn-b'])
  })
})
