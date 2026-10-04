// api.js — klien HTTP dashboard: sesi cookie + CSRF double-submit.
(function () {
  let csrfToken = sessionStorage.getItem('jr_csrf') || '';

  async function request(method, path, body, opts = {}) {
    const headers = { 'Content-Type': 'application/json' };
    if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
    const res = await fetch(path, {
      method,
      headers,
      credentials: 'same-origin',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    let data = null;
    try { data = await res.json(); } catch (e) { /* bukan JSON */ }
    if (!res.ok) {
      if (res.status === 401 && !opts.noRedirect && !path.startsWith('/v1')) {
        // sesi mati → kembali ke login
        if (window.App) App.onUnauthorized();
      }
      const err = new Error((data && (data.error || data.message)) || ('HTTP ' + res.status));
      err.status = res.status;
      err.data = data;
      throw err;
    }
    return data;
  }

  window.API = {
    get: (p, opts) => request('GET', p, undefined, opts),
    post: (p, b, opts) => request('POST', p, b, opts),
    put: (p, b, opts) => request('PUT', p, b, opts),
    patch: (p, b, opts) => request('PATCH', p, b, opts),
    del: (p, opts) => request('DELETE', p, undefined, opts),
    setCSRF(t) { csrfToken = t || ''; sessionStorage.setItem('jr_csrf', csrfToken); },
    csrf: () => csrfToken,
    reset() { csrfToken = ''; sessionStorage.removeItem('jr_csrf'); },

    // inferensi playground (auth via cookie tidak berlaku di /v1 — pakai
    // chat proxy khusus member di bawah ini)
    async chatStream({ model, messages, temperature, maxTokens, onDelta, onMeta, signal }) {
      const res = await fetch('/api/me/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
        credentials: 'same-origin',
        body: JSON.stringify({ model, messages, temperature, max_tokens: maxTokens, stream: true }),
        signal,
      });
      if (!res.ok) {
        let msg = 'HTTP ' + res.status;
        try { const j = await res.json(); msg = j.error || msg; } catch (e) {}
        throw new Error(msg);
      }
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        const events = buf.split('\n\n');
        buf = events.pop();
        for (const ev of events) {
          for (const line of ev.split('\n')) {
            if (!line.startsWith('data:')) continue;
            const payload = line.slice(5).trim();
            if (payload === '[DONE]') return;
            try {
              const j = JSON.parse(payload);
              const delta = j.choices && j.choices[0] && j.choices[0].delta;
              if (delta && delta.content) onDelta(delta.content);
              if (delta && delta.tool_calls) onDelta('');
              if (j.usage && onMeta) onMeta(j.usage);
              if (j.error) throw new Error(j.error.message || 'upstream error');
            } catch (e) {
              if (e.message && !/JSON/.test(e.message)) throw e;
            }
          }
        }
      }
    },
  };
})();
