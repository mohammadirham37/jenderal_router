// ui.js — helper DOM ringan: elemen, tabel, toast, modal, render markdown.
(function () {
  function el(tag, attrs = {}, children = []) {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === 'class') node.className = v;
      else if (k === 'html') node.innerHTML = v;
      else if (k.startsWith('on') && typeof v === 'function') node.addEventListener(k.slice(2), v);
      else if (v !== undefined && v !== null) node.setAttribute(k, v);
    }
    for (const c of [].concat(children)) {
      if (c === null || c === undefined) continue;
      node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    }
    return node;
  }

  function toast(msg, kind = 'info', ms = 4000) {
    const t = el('div', { class: 'toast ' + kind }, [msg]);
    document.getElementById('toasts').appendChild(t);
    setTimeout(() => t.remove(), ms);
  }

  function modal(contentEl) {
    const root = document.getElementById('modal-root');
    const back = el('div', { class: 'modal-back' }, [el('div', { class: 'modal' }, [contentEl])]);
    back.addEventListener('click', (e) => { if (e.target === back) back.remove(); });
    root.appendChild(back);
    return () => back.remove();
  }

  function fmtNum(n) {
    if (n === undefined || n === null) return '0';
    return new Intl.NumberFormat('id-ID').format(n);
  }
  function fmtCost(v) {
    const f = parseFloat(v || 0);
    return '$' + f.toFixed(4);
  }
  function fmtMs(v) { return (v || 0) + ' ms'; }
  function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  // markdown minimal: heading, bold, italic, inline code, fenced code, list, link
  function md(src) {
    const lines = String(src || '').split('\n');
    let html = '', inCode = false, codeBuf = '', inList = false;
    const closeList = () => { if (inList) { html += '</ul>'; inList = false; } };
    const inline = (s) => esc(s)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\*([^*]+)\*/g, '<em>$1</em>')
      .replace(/\[([^\]]+)\]\((https?:[^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    for (const raw of lines) {
      if (raw.trim().startsWith('```')) {
        if (inCode) { html += '<pre><code>' + esc(codeBuf) + '</code></pre>'; codeBuf = ''; inCode = false; }
        else { closeList(); inCode = true; }
        continue;
      }
      if (inCode) { codeBuf += raw + '\n'; continue; }
      const line = raw;
      if (/^#{1,4}\s/.test(line)) { closeList(); html += '<p><strong>' + inline(line.replace(/^#+\s/, '')) + '</strong></p>'; }
      else if (/^[-*]\s/.test(line)) { if (!inList) { html += '<ul>'; inList = true; } html += '<li>' + inline(line.replace(/^[-*]\s/, '')) + '</li>'; }
      else if (line.trim() === '') { closeList(); }
      else { closeList(); html += '<p>' + inline(line) + '</p>'; }
    }
    if (inCode) html += '<pre><code>' + esc(codeBuf) + '</code></pre>';
    closeList();
    return html || '<p></p>';
  }

  function statCard(label, value, sub) {
    return el('div', { class: 'card' }, [el('div', { class: 'stat' }, [
      el('span', { class: 'lbl' }, [label]),
      el('span', { class: 'val' }, [value]),
      sub ? el('span', { class: 'muted small' }, [sub]) : null,
    ])]);
  }

  function badge(text, kind) { return el('span', { class: 'badge ' + (kind || 'info') }, [text]); }

  window.UI = { el, toast, modal, fmtNum, fmtCost, fmtMs, esc, md, statCard, badge };
})();
