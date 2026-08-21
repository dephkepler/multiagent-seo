'use client'

// navigator.clipboard exists only in a secure context, and prod is served over
// plain HTTP on an IP — there the API is simply absent, which is why the copy
// button used to fail with "Clipboard unavailable" and passwords had to be
// retyped by hand. execCommand('copy') is deprecated but it is what works
// there, so it stays as the fallback.
export async function copyText(value: string): Promise<boolean> {
  try {
    if (window.isSecureContext && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return true
    }
  } catch {
    // fall through to the textarea trick
  }

  try {
    const ta = document.createElement('textarea')
    ta.value = value
    ta.setAttribute('readonly', '')
    // Off-screen but still focusable: a hidden or display:none element cannot
    // be selected, and the copy would quietly do nothing.
    ta.style.position = 'fixed'
    ta.style.top = '-1000px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    ta.setSelectionRange(0, value.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
