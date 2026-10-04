// pages-chat.js — chat playground (F-12): streaming, riwayat, kuota.
(function () {
  const { el, toast, fmtNum, md, esc } = UI;
  const t = (k) => I18N.t(k);

  let currentConv = null;
  let conversations = [];
  let modelChoices = [];

  Pages.chat = async function (main) {
    main.innerHTML = '';
    main.appendChild(el('h1', {}, [t('chat')]));
    const layout = el('div', { class: 'chat-layout' });
    main.appendChild(layout);

    // kolom kiri: percakapan
    const left = el('div', { class: 'card' });
    left.appendChild(el('button', { class: 'btn', style: 'width:100%;margin-bottom:10px', onclick: newConversation }, ['+ ' + t('new_chat')]));
    const convList = el('div', { class: 'conv-list' });
    left.appendChild(convList);
    const quotaBox = el('div', { class: 'muted small', style: 'margin-top:10px;border-top:1px solid var(--border);padding-top:8px' });
    left.appendChild(quotaBox);
    layout.appendChild(left);

    // kolom kanan: chat
    const right = el('div', { class: 'card chat-pane' });
    const modelSel = el('select', { style: 'max-width:280px' });
    const settingsRow = el('div', { class: 'kv', style: 'margin-bottom:8px' }, [modelSel]);
    right.appendChild(settingsRow);
    const msgs = el('div', { class: 'chat-msgs' });
    right.appendChild(msgs);
    const input = el('textarea', { placeholder: 'Tulis pesan… (Enter=kirim, Shift+Enter=baris baru)' });
    const sendBtn = el('button', { class: 'btn', onclick: () => send() }, [t('send')]);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
    });
    right.appendChild(el('div', { class: 'chat-input-row' }, [input, sendBtn]));
    layout.appendChild(right);

    await Promise.all([loadModels(), loadConversations(), loadQuota()]);

    async function loadModels() {
      try {
        const res = await API.get('/api/me/models');
        modelChoices = res.models || [];
        modelSel.innerHTML = '';
        for (const m of modelChoices) modelSel.appendChild(el('option', { value: m.id }, [m.id]));
      } catch (e) { toast(e.message, 'err'); }
    }

    async function loadQuota() {
      try {
        const res = await API.get('/api/me/usage');
        quotaBox.innerHTML = '';
        quotaBox.appendChild(el('div', {}, [t('quota_left') + ':']));
        for (const q of res.quotas || []) {
          const pct = q.token_limit ? Math.min(100, Math.round((q.used_tokens / q.token_limit) * 100)) : 0;
          quotaBox.appendChild(el('div', {}, [
            `${q.period}: ${fmtNum(q.used_tokens)}/${fmtNum(q.token_limit)} (${pct}%)`,
          ]));
        }
        quotaBox.appendChild(el('div', { class: 'muted' }, [`${t('requests')}: ${fmtNum(res.today ? res.today.requests : 0)}`]));
      } catch (e) { /* abaikan */ }
    }

    async function loadConversations() {
      try {
        const res = await API.get('/api/me/conversations');
        conversations = res.conversations || [];
        convList.innerHTML = '';
        for (const c of conversations) {
          const item = el('div', { class: 'conv-item' + (currentConv === c.id ? ' active' : ''), onclick: () => openConversation(c.id) }, [
            el('span', {}, [c.title || '…']),
            el('button', { class: 'btn ghost small', onclick: async (e) => {
              e.stopPropagation();
              if (!confirm(t('confirm_delete'))) return;
              await API.del('/api/me/conversations/' + c.id);
              if (currentConv === c.id) { currentConv = null; msgs.innerHTML = ''; }
              loadConversations();
            } }, ['×']),
          ]);
          convList.appendChild(item);
        }
      } catch (e) { toast(e.message, 'err'); }
    }

    async function newConversation() {
      const model = modelSel.value || '';
      const res = await API.post('/api/me/conversations', { title: t('new_chat'), model });
      currentConv = res.conversation.id;
      msgs.innerHTML = '';
      loadConversations();
    }

    async function openConversation(id) {
      currentConv = id;
      const res = await API.get('/api/me/conversations/' + id);
      msgs.innerHTML = '';
      for (const m of res.messages || []) addMsg(m.role, m.content, m.provider_name);
      loadConversations();
    }

    function addMsg(role, content, provider) {
      const bubble = el('div', { class: 'msg ' + role });
      if (role === 'assistant') {
        bubble.innerHTML = md(content);
        if (provider) bubble.appendChild(el('div', { class: 'meta' }, [t('answered_by') + ': ' + provider]));
      } else {
        bubble.innerHTML = md(content);
      }
      msgs.appendChild(bubble);
      msgs.scrollTop = msgs.scrollHeight;
      return bubble;
    }

    let sending = false;
    async function send() {
      const text = input.value.trim();
      if (!text || sending) return;
      if (!currentConv) await newConversation();
      sending = true;
      sendBtn.disabled = true;
      input.value = '';
      addMsg('user', text);

      // kumpulkan riwayat yang terlihat
      const history = [];
      msgs.querySelectorAll('.msg').forEach((m) => {
        history.push({ role: m.classList.contains('user') ? 'user' : 'assistant', content: m.textContent });
      });

      const bubble = addMsg('assistant', '…');
      let acc = '';
      try {
        await API.chatStream({
          model: modelSel.value,
          messages: history.slice(0, -1).concat([{ role: 'user', content: text }]),
          onDelta: (d) => {
            acc += d;
            bubble.innerHTML = md(acc) || '…';
            msgs.scrollTop = msgs.scrollHeight;
          },
          onMeta: () => loadQuota(),
        });
        if (!acc) acc = '(kosong)';
        bubble.innerHTML = md(acc);
        await API.post('/api/me/conversations/' + currentConv + '/messages', { role: 'user', content: text });
        await API.post('/api/me/conversations/' + currentConv + '/messages', { role: 'assistant', content: acc, model: modelSel.value });
        loadQuota();
        loadConversations();
      } catch (e) {
        bubble.innerHTML = md(acc + '\n\n**⚠ ' + esc(e.message) + '**');
      } finally {
        sending = false;
        sendBtn.disabled = false;
      }
    }
  };

  window.Pages = window.Pages || {};
  window.Pages.chat = Pages.chat;
})();
