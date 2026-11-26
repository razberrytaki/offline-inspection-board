type Field = HTMLInputElement | HTMLTextAreaElement;

export function observeValidation(announce: (message: string) => void) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let announcedField: Field | undefined;

  function sync(form: HTMLFormElement) {
    for (const field of form.querySelectorAll<Field | HTMLFieldSetElement>('[data-error]')) {
      const invalid = field instanceof HTMLFieldSetElement
        ? field.matches(':has(input:user-invalid)')
        : field.matches(':user-invalid');
      if (field.hasAttribute('aria-invalid')) field.setAttribute('aria-invalid', String(invalid));
      const errorId = field.dataset.error;
      if (invalid && errorId !== undefined) field.setAttribute('aria-describedby', errorId);
      else field.removeAttribute('aria-describedby');
    }
    if (announcedField?.form === form && !announcedField.matches(':user-invalid')) {
      clearTimeout(timer);
      announcedField = undefined;
      announce('');
    }
  }

  function update(event: Event) {
    const field = event.target;
    if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement) || !field.form) return;
    const form = field.form;
    sync(form);
    if (event.type === 'change' || event.type === 'input') queueMicrotask(() => sync(form));
    if (event.type === 'blur' && field.matches(':user-invalid') && field !== announcedField) {
      const errorId = field.dataset.error;
      if (!errorId) return;
      clearTimeout(timer);
      announce('');
      announcedField = field;
      timer = setTimeout(() => {
        if (field.isConnected && field.matches(':user-invalid')) {
          announce(document.getElementById(errorId)?.textContent?.trim() ?? '');
        }
      }, 500);
    }
  }

  function reset(event: Event) {
    const form = event.target;
    if (form instanceof HTMLFormElement) requestAnimationFrame(() => sync(form));
  }

  const events = ['blur', 'input', 'change', 'invalid'];
  for (const event of events) document.addEventListener(event, update, true);
  document.addEventListener('reset', reset, true);
  return () => {
    clearTimeout(timer);
    for (const event of events) document.removeEventListener(event, update, true);
    document.removeEventListener('reset', reset, true);
  };
}
