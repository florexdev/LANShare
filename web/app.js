// LANShare Modern Frontend Logic
(function () {
  let ws = null;
  let activePeers = new Map();
  let selectedPeerId = null;
  let currentConfig = null;
  let activeTransfers = new Map();

  // DOM Elements
  const deviceSub = document.getElementById('device-info-subtitle');
  const selfNameEl = document.getElementById('self-device-name');
  const selfIpEl = document.getElementById('self-ip');
  const selfOsIconEl = document.getElementById('self-os-icon');
  const radarStatusTag = document.getElementById('radar-status-tag');
  const peerGrid = document.getElementById('peer-grid');
  const dropZone = document.getElementById('drop-zone');
  const fileInput = document.getElementById('file-input');
  const folderInput = document.getElementById('folder-input');
  const activeTransfersPanel = document.getElementById('active-transfers');
  const transfersList = document.getElementById('transfers-list');
  const historyTbody = document.getElementById('history-tbody');
  const historyEmpty = document.getElementById('history-empty');
  const historyCountBadge = document.getElementById('history-count');
  const clearHistoryBtn = document.getElementById('clear-history-btn');

  // Modal Elements
  const incomingModal = document.getElementById('incoming-modal');
  const modalPeerName = document.getElementById('modal-peer-name');
  const modalFileName = document.getElementById('modal-file-name');
  const modalFileSize = document.getElementById('modal-file-size');
  const modalAcceptBtn = document.getElementById('modal-accept-btn');
  const modalDeclineBtn = document.getElementById('modal-decline-btn');
  let currentPromptSessionId = null;

  // Settings Elements
  const settingsForm = document.getElementById('settings-form');
  const settingLang = document.getElementById('setting-lang');
  const settingName = document.getElementById('setting-name');
  const settingDir = document.getElementById('setting-dir');
  const settingE2ee = document.getElementById('setting-e2ee');
  const settingAutoAccept = document.getElementById('setting-auto-accept');

  // Theme Elements
  const themeToggleBtn = document.getElementById('theme-toggle-btn');
  const themeIcon = document.getElementById('theme-icon');
  const themeLabel = document.getElementById('theme-label');

  let currentLang = 'tr';

  const translations = {
    tr: {
      nav_radar: '📡 Radar',
      nav_history: '📜 Geçmiş',
      nav_settings: '⚙️ Ayarlar',
      searching_devices: '📡 Yakındaki cihazlar aranıyor...',
      devices_online: 'cihaz çevrimiçi',
      drop_title: 'Dosya veya Klasörleri Buraya Bırakın',
      drop_subtitle: 'Yukarıdan hedef bir cihaz seçin veya dosyaları buraya sürükleyin',
      btn_browse_files: '📄 Dosya Seç',
      btn_select_folder: '📁 Klasör Seç',
      live_transfers: '⚡ Canlı Transferler',
      history_title: '📜 Transfer Geçmişi',
      history_subtitle: 'Gelen ve giden tüm dosyaların kayıtları',
      btn_open_downloads: '📁 İndirilenler Klasörünü Aç',
      btn_clear_history: '🗑️ Geçmişi Temizle',
      col_file: 'Dosya / Klasör',
      col_dir: 'Yön',
      col_device: 'Karşı Cihaz',
      col_size: 'Boyut',
      col_speed: 'Hız',
      col_status: 'Durum',
      col_date: 'Tarih & Saat',
      col_action: 'İşlem',
      empty_history: 'Henüz kaydedilmiş transfer geçmişi yok.',
      settings_title: '⚙️ Cihaz ve Sistem Ayarları',
      settings_subtitle: 'LANShare davranışını ve tercihlerini özelleştirin',
      label_lang: '🌐 Dil / Language',
      label_name: 'Cihaz Takma Adı (Nickname)',
      help_name: 'Bu isim yakındaki ağ cihazlarına yayınlanacaktır.',
      label_dir: 'Varsayılan İndirme Klasörü',
      help_dir: 'Gelen dosyaların diskinize kaydedileceği yer.',
      label_e2ee: '🔒 Uçtan Uca Şifreleme (E2EE)',
      help_e2ee: 'Aktarılan veriyi AES-256-GCM ile şifreler.',
      label_auto_accept: '⚡ Transferleri Otomatik Kabul Et',
      help_auto_accept: 'Gelen dosya isteklerini onay penceresi sormadan kabul eder.',
      btn_save_settings: '💾 Ayarları Kaydet',
      modal_incoming: '📲 Gelen Dosya İsteği',
      modal_wants_send: 'size bir dosya göndermek istiyor',
      btn_decline: '❌ Reddet',
      btn_accept: '✅ Kabul Et & İndir',
      outgoing: '📤 Giden',
      incoming: '📥 Gelen',
      open_folder: '📁 Klasörü Aç'
    },
    en: {
      nav_radar: '📡 Radar',
      nav_history: '📜 History',
      nav_settings: '⚙️ Settings',
      searching_devices: '📡 Searching for nearby devices...',
      devices_online: 'device(s) online',
      drop_title: 'Drop Files or Folders Here',
      drop_subtitle: 'Select a target peer above or drop files into the slot',
      btn_browse_files: '📄 Browse Files',
      btn_select_folder: '📁 Select Folder',
      live_transfers: '⚡ Live Transfers',
      history_title: '📜 Transfer Log',
      history_subtitle: 'Complete record of incoming and outgoing files',
      btn_open_downloads: '📁 Open Downloads Folder',
      btn_clear_history: '🗑️ Clear History',
      col_file: 'File / Folder',
      col_dir: 'Direction',
      col_device: 'Peer Device',
      col_size: 'Size',
      col_speed: 'Speed',
      col_status: 'Status',
      col_date: 'Date & Time',
      col_action: 'Action',
      empty_history: 'No transfer history recorded yet.',
      settings_title: '⚙️ Device & System Settings',
      settings_subtitle: 'Customize LANShare behavior on this machine',
      label_lang: '🌐 Language / Dil',
      label_name: 'Device Nickname',
      help_name: 'This name will be broadcasted to nearby LAN devices.',
      label_dir: 'Default Download Directory',
      help_dir: 'Where received files are saved on your disk.',
      label_e2ee: '🔒 End-to-End Encryption (E2EE)',
      help_e2ee: 'Encrypt payload using AES-256-GCM before sending.',
      label_auto_accept: '⚡ Auto-Accept Transfers',
      help_auto_accept: 'Automatically accept incoming file requests.',
      btn_save_settings: '💾 Save Settings',
      modal_incoming: '📲 Incoming File Request',
      modal_wants_send: 'wants to send you a file',
      btn_decline: '❌ Decline',
      btn_accept: '✅ Accept & Receive',
      outgoing: '📤 Outgoing',
      incoming: '📥 Incoming',
      open_folder: '📁 Open Folder'
    }
  };

  // Initialize
  document.addEventListener('DOMContentLoaded', () => {
    initLanguage();
    initTheme();
    initNavigation();
    initWebSocket();
    loadConfig();
    loadPeers();
    loadHistory();
    initDragAndDrop();
    initFormHandlers();
  });

  function initLanguage() {
    currentLang = localStorage.getItem('lanshare_lang') || 'tr';
    if (settingLang) {
      settingLang.value = currentLang;
      settingLang.addEventListener('change', (e) => {
        applyLanguage(e.target.value);
      });
    }
    applyLanguage(currentLang);
  }

  function applyLanguage(lang) {
    currentLang = lang || 'tr';
    localStorage.setItem('lanshare_lang', currentLang);
    const t = translations[currentLang] || translations.tr;

    // Navigation
    const navRadar = document.getElementById('txt-nav-radar');
    if (navRadar) navRadar.textContent = t.nav_radar;

    const navSettings = document.getElementById('txt-nav-settings');
    if (navSettings) navSettings.textContent = t.nav_settings;

    const navHistory = document.getElementById('txt-nav-history');
    if (navHistory) {
      const badge = document.getElementById('history-count');
      navHistory.innerHTML = `${t.nav_history} <span class="badge" id="history-count">${badge ? badge.textContent : '0'}</span>`;
    }

    // Dropzone
    const dropTitle = document.getElementById('txt-drop-title');
    if (dropTitle) dropTitle.textContent = t.drop_title;

    const dropSub = document.getElementById('txt-drop-subtitle');
    if (dropSub) dropSub.textContent = t.drop_subtitle;

    const btnBrowse = document.getElementById('txt-btn-browse');
    if (btnBrowse) btnBrowse.textContent = t.btn_browse_files;

    const btnFolder = document.getElementById('txt-btn-folder');
    if (btnFolder) btnFolder.textContent = t.btn_select_folder;

    const liveTransfers = document.getElementById('txt-live-transfers');
    if (liveTransfers) liveTransfers.textContent = t.live_transfers;

    // History Tab
    const historyTitle = document.getElementById('txt-history-title');
    if (historyTitle) historyTitle.textContent = t.history_title;

    const historySub = document.getElementById('txt-history-subtitle');
    if (historySub) historySub.textContent = t.history_subtitle;

    const openDownloadsBtn = document.getElementById('open-downloads-btn');
    if (openDownloadsBtn) openDownloadsBtn.innerHTML = t.btn_open_downloads;

    const clearHistoryBtn = document.getElementById('clear-history-btn');
    if (clearHistoryBtn) clearHistoryBtn.innerHTML = t.btn_clear_history;

    const thFile = document.getElementById('th-file');
    if (thFile) thFile.textContent = t.col_file;

    const thDir = document.getElementById('th-direction');
    if (thDir) thDir.textContent = t.col_dir;

    const thPeer = document.getElementById('th-peer');
    if (thPeer) thPeer.textContent = t.col_device;

    const thSize = document.getElementById('th-size');
    if (thSize) thSize.textContent = t.col_size;

    const thSpeed = document.getElementById('th-speed');
    if (thSpeed) thSpeed.textContent = t.col_speed;

    const thStatus = document.getElementById('th-status');
    if (thStatus) thStatus.textContent = t.col_status;

    const thDate = document.getElementById('th-date');
    if (thDate) thDate.textContent = t.col_date;

    const thAction = document.getElementById('th-action');
    if (thAction) thAction.textContent = t.col_action;

    const historyEmpty = document.getElementById('txt-history-empty');
    if (historyEmpty) historyEmpty.textContent = t.empty_history;

    // Settings Tab
    const settingsTitle = document.getElementById('txt-settings-title');
    if (settingsTitle) settingsTitle.textContent = t.settings_title;

    const settingsSub = document.getElementById('txt-settings-subtitle');
    if (settingsSub) settingsSub.textContent = t.settings_subtitle;

    const labelLang = document.getElementById('txt-label-lang');
    if (labelLang) labelLang.textContent = t.label_lang;

    const labelName = document.getElementById('txt-label-name');
    if (labelName) labelName.textContent = t.label_name;

    const helpName = document.getElementById('txt-help-name');
    if (helpName) helpName.textContent = t.help_name;

    const labelDir = document.getElementById('txt-label-dir');
    if (labelDir) labelDir.textContent = t.label_dir;

    const helpDir = document.getElementById('txt-help-dir');
    if (helpDir) helpDir.textContent = t.help_dir;

    const settingOpenDirBtn = document.getElementById('setting-open-dir-btn');
    if (settingOpenDirBtn) settingOpenDirBtn.innerHTML = t.open_folder;

    const labelE2ee = document.getElementById('txt-label-e2ee');
    if (labelE2ee) labelE2ee.textContent = t.label_e2ee;

    const helpE2ee = document.getElementById('txt-help-e2ee');
    if (helpE2ee) helpE2ee.textContent = t.help_e2ee;

    const labelAutoAccept = document.getElementById('txt-label-auto-accept');
    if (labelAutoAccept) labelAutoAccept.textContent = t.label_auto_accept;

    const helpAutoAccept = document.getElementById('txt-help-auto-accept');
    if (helpAutoAccept) helpAutoAccept.textContent = t.help_auto_accept;

    const btnSave = document.getElementById('txt-btn-save');
    if (btnSave) btnSave.innerHTML = t.btn_save_settings;

    // Modal
    const modalBadge = document.getElementById('txt-modal-badge');
    if (modalBadge) modalBadge.textContent = t.modal_incoming;

    const modalSub = document.getElementById('modal-peer-sub');
    if (modalSub) modalSub.textContent = t.modal_wants_send;

    const modalDeclineBtn = document.getElementById('modal-decline-btn');
    if (modalDeclineBtn) modalDeclineBtn.innerHTML = t.btn_decline;

    const modalAcceptBtn = document.getElementById('modal-accept-btn');
    if (modalAcceptBtn) modalAcceptBtn.innerHTML = t.btn_accept;

    renderPeers();
    loadHistory();
  }

  // Theme Management
  function initTheme() {
    const savedTheme = localStorage.getItem('lanshare_theme') || 'dark';
    setTheme(savedTheme);

    if (themeToggleBtn) {
      themeToggleBtn.addEventListener('click', () => {
        const currentTheme = document.documentElement.getAttribute('data-theme') || 'dark';
        const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
        setTheme(newTheme);
      });
    }
  }

  function setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('lanshare_theme', theme);

    if (themeIcon && themeLabel) {
      if (theme === 'light') {
        themeIcon.textContent = '☀️';
        themeLabel.textContent = 'Light';
      } else {
        themeIcon.textContent = '🌙';
        themeLabel.textContent = 'Dark';
      }
    }
  }

  // Tab Navigation
  function initNavigation() {
    document.querySelectorAll('.nav-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));

        btn.classList.add('active');
        const tabId = btn.getAttribute('data-tab');
        document.getElementById(tabId).classList.add('active');

        if (tabId === 'tab-history') {
          loadHistory();
        }
      });
    });
  }

  // WebSocket Connection & Event Handling
  function initWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      console.log('[LANShare] WebSocket connected');
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        handleWSEvent(msg.event, msg.data);
      } catch (e) {
        console.error('Error parsing WS message', e);
      }
    };

    ws.onclose = () => {
      console.log('[LANShare] WebSocket connection closed, retrying in 3s...');
      setTimeout(initWebSocket, 3000);
    };
  }

  function handleWSEvent(event, data) {
    switch (event) {
      case 'peer_event':
        handlePeerEvent(data);
        break;
      case 'transfer_prompt':
        showIncomingModal(data);
        break;
      case 'transfer_progress':
        updateTransferProgress(data);
        break;
      case 'history_updated':
      case 'history_cleared':
        loadHistory();
        break;
      case 'config_updated':
        currentConfig = data;
        renderConfig();
        break;
    }
  }

  // Configuration Management
  async function loadConfig() {
    try {
      const res = await fetch('/api/config');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      currentConfig = await res.json();
      renderConfig();
    } catch (e) {
      console.warn('[LANShare] Go backend API not reached. Ensure Go executable is running at http://localhost:52639', e);
    }
  }

  function renderConfig() {
    if (!currentConfig) return;
    selfNameEl.textContent = currentConfig.device_name;
    deviceSub.textContent = `${currentConfig.device_name} (${getOSLabel(currentConfig.os)})`;
    selfOsIconEl.textContent = getOSIcon(currentConfig.os);

    settingName.value = currentConfig.device_name || '';
    settingDir.value = currentConfig.download_dir || '';
    settingE2ee.checked = !!currentConfig.e2ee_enabled;
    settingAutoAccept.checked = !!currentConfig.auto_accept;
  }

  function initFormHandlers() {
    settingsForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const payload = {
        device_name: settingName.value,
        download_dir: settingDir.value,
        e2ee_enabled: settingE2ee.checked,
        auto_accept: settingAutoAccept.checked
      };

      try {
        const res = await fetch('/api/config', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || `Server response status ${res.status}`);
        }
        currentConfig = await res.json();
        renderConfig();
        alert('Settings updated successfully!');
      } catch (err) {
        alert('Hata: Go arka plan sunucusuna erişilemedi! Lütfen VS Code Live Server (5500) yerine LANShare uygulamasını çalıştırıp http://localhost:52639 adresinden girin.\n\nDetay: ' + err.message);
      }
    });

    clearHistoryBtn.addEventListener('click', async () => {
      if (confirm('Are you sure you want to clear all transfer history?')) {
        await fetch('/api/history', { method: 'DELETE' });
        loadHistory();
      }
    });

    modalAcceptBtn.addEventListener('click', () => respondPrompt(true));
    modalDeclineBtn.addEventListener('click', () => respondPrompt(false));

    const openDownloadsBtn = document.getElementById('open-downloads-btn');
    if (openDownloadsBtn) {
      openDownloadsBtn.addEventListener('click', () => window.LANShareOpenFolder(''));
    }

    const settingOpenDirBtn = document.getElementById('setting-open-dir-btn');
    if (settingOpenDirBtn) {
      settingOpenDirBtn.addEventListener('click', () => window.LANShareOpenFolder(settingDir.value || ''));
    }
  }

  // Peer Discovery
  async function loadPeers() {
    try {
      const res = await fetch('/api/peers');
      if (!res.ok) return;
      const peers = await res.json();
      activePeers.clear();
      peers.forEach(p => activePeers.set(p.id, p));
      renderPeers();
    } catch (e) {
      console.warn('Failed to fetch peers', e);
    }
  }

  function handlePeerEvent(data) {
    if (data.type === 'joined' || data.type === 'updated') {
      activePeers.set(data.peer.id, data.peer);
    } else if (data.type === 'left') {
      activePeers.delete(data.peer.id);
      if (selectedPeerId === data.peer.id) {
        selectedPeerId = null;
      }
    }
    renderPeers();
  }

  function renderPeers() {
    peerGrid.innerHTML = '';
    const t = translations[currentLang] || translations.tr;

    if (activePeers.size === 0) {
      if (radarStatusTag) {
        radarStatusTag.textContent = t.searching_devices;
      }
      return;
    }

    if (radarStatusTag) {
      radarStatusTag.textContent = `🟢 ${activePeers.size} ${t.devices_online}`;
    }

    activePeers.forEach(peer => {
      const card = document.createElement('div');
      card.className = `peer-card ${selectedPeerId === peer.id ? 'selected' : ''}`;
      card.innerHTML = `
        <div class="peer-avatar">${getOSIcon(peer.os)}</div>
        <div class="peer-info">
          <span class="peer-name">${escapeHTML(peer.name)}</span>
          <span class="peer-meta">${peer.ip} &bull; ${peer.e2ee ? '🔒 E2EE' : 'Plain'}</span>
        </div>
      `;

      card.addEventListener('click', () => {
        if (selectedPeerId === peer.id) {
          selectedPeerId = null;
          card.classList.remove('selected');
        } else {
          document.querySelectorAll('.peer-card').forEach(c => c.classList.remove('selected'));
          selectedPeerId = peer.id;
          card.classList.add('selected');
        }
      });

      peerGrid.appendChild(card);
    });
  }

  // Drag & Drop
  function initDragAndDrop() {
    ['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
      dropZone.addEventListener(eventName, preventDefaults, false);
      document.body.addEventListener(eventName, preventDefaults, false);
    });

    ['dragenter', 'dragover'].forEach(eventName => {
      dropZone.addEventListener(eventName, () => dropZone.classList.add('dragover'), false);
    });

    ['dragleave', 'drop'].forEach(eventName => {
      dropZone.addEventListener(eventName, () => dropZone.classList.remove('dragover'), false);
    });

    dropZone.addEventListener('drop', handleDrop, false);
    fileInput.addEventListener('change', (e) => handleFileSelect(e.target.files));
    folderInput.addEventListener('change', (e) => handleFileSelect(e.target.files));
  }

  function preventDefaults(e) {
    e.preventDefault();
    e.stopPropagation();
  }

  function handleDrop(e) {
    const dt = e.dataTransfer;
    const files = dt.files;
    handleFileSelect(files);
  }

  async function handleFileSelect(files) {
    if (!files || files.length === 0) return;

    if (!selectedPeerId) {
      // Pick first available peer if none selected
      if (activePeers.size === 0) {
        alert('No nearby LAN devices detected! Please ensure LANShare is running on the target device.');
        return;
      }
      const firstPeer = activePeers.values().next().value;
      selectedPeerId = firstPeer.id;
    }

    const peer = activePeers.get(selectedPeerId);
    if (!peer) return;

    const file = files[0];

    // If running in desktop container with direct file.path access
    if (file.path) {
      try {
        const res = await fetch('/api/send', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            peer_ip: peer.ip,
            peer_port: peer.port,
            peer_id: peer.id,
            peer_name: peer.name,
            peer_os: peer.os,
            local_path: file.path
          })
        });

        if (!res.ok) {
          const errText = await res.text();
          alert('Failed to send: ' + errText);
        }
      } catch (err) {
        alert('Send error: ' + err.message);
      }
      return;
    }

    // Standard Web Browser Sandbox: Stream file binary blob directly via /api/send_web
    try {
      const formData = new FormData();
      formData.append('peer_ip', peer.ip);
      formData.append('peer_port', peer.port);
      formData.append('peer_id', peer.id);
      formData.append('peer_name', peer.name);
      formData.append('peer_os', peer.os);
      formData.append('file_name', file.name);
      formData.append('file', file);

      const res = await fetch('/api/send_web', {
        method: 'POST',
        body: formData
      });

      if (!res.ok) {
        const errText = await res.text();
        alert('Failed to send: ' + errText);
      }
    } catch (err) {
      alert('Send error: ' + err.message);
    }
  }

  // Incoming Prompt Modal
  function showIncomingModal(manifest) {
    currentPromptSessionId = manifest.session_id;
    modalPeerName.textContent = manifest.sender_name;
    modalFileName.textContent = manifest.root_name;
    modalFileSize.textContent = formatBytes(manifest.total_size);
    incomingModal.classList.remove('hidden');
  }

  async function respondPrompt(accept) {
    if (!currentPromptSessionId) return;

    try {
      await fetch('/api/action', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: currentPromptSessionId,
          action: accept ? 'accept' : 'decline'
        })
      });
    } catch (e) {
      console.error('Failed to respond to prompt', e);
    } finally {
      incomingModal.classList.add('hidden');
      currentPromptSessionId = null;
    }
  }

  // Real-Time Progress Cards
  function updateTransferProgress(prog) {
    activeTransfersPanel.classList.remove('hidden');
    activeTransfers.set(prog.session_id, prog);

    let card = document.getElementById(`tx-card-${prog.session_id}`);
    if (!card) {
      card = document.createElement('div');
      card.id = `tx-card-${prog.session_id}`;
      card.className = 'transfer-card';
      transfersList.appendChild(card);
    }

    const pct = prog.progress_pct.toFixed(1);
    const speedStr = prog.current_speed ? `${prog.current_speed.toFixed(1)} MB/s` : '0.0 MB/s';
    const etaStr = prog.eta_seconds ? `${prog.eta_seconds}s remaining` : '';

    card.innerHTML = `
      <div class="transfer-card-header">
        <span class="transfer-file-title">${escapeHTML(prog.current_file || 'Transfer')}</span>
        <span class="transfer-speed-badge">${speedStr}</span>
      </div>
      <div class="progress-bar-bg">
        <div class="progress-bar-fill" style="width: ${pct}%"></div>
      </div>
      <div class="transfer-card-footer">
        <span>${formatBytes(prog.bytes_sent)} / ${formatBytes(prog.total_bytes)} (${pct}%)</span>
        <span>${etaStr}</span>
        <div class="transfer-controls">
          ${prog.status === 'paused' ? 
            `<button class="control-btn" onclick="window.LANShareAction('${prog.session_id}', 'resume')">Resume</button>` :
            `<button class="control-btn" onclick="window.LANShareAction('${prog.session_id}', 'pause')">Pause</button>`}
          <button class="control-btn" onclick="window.LANShareAction('${prog.session_id}', 'cancel')">Cancel</button>
        </div>
      </div>
    `;

    if (prog.status === 'completed' || prog.status === 'failed' || prog.status === 'cancelled') {
      setTimeout(() => {
        card.remove();
        activeTransfers.delete(prog.session_id);
        if (activeTransfers.size === 0) {
          activeTransfersPanel.classList.add('hidden');
        }
        loadHistory();
      }, 3000);
    }
  }

  window.LANShareAction = async function (sessionId, action) {
    await fetch('/api/action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionId, action: action })
    });
  };

  // Transfer History
  async function loadHistory() {
    try {
      const res = await fetch('/api/history');
      const records = await res.json();
      renderHistory(records);
    } catch (e) {
      console.error('Failed to load history', e);
    }
  }

  function renderHistory(records) {
    historyTbody.innerHTML = '';
    historyCountBadge.textContent = records.length;

    if (!records || records.length === 0) {
      historyEmpty.style.display = 'block';
      return;
    }

    historyEmpty.style.display = 'none';

    records.forEach(rec => {
      const t = translations[currentLang] || translations.tr;
      const tr = document.createElement('tr');
      const dt = new Date(rec.timestamp).toLocaleString();
      const dirIcon = rec.direction === 'outgoing' ? `${t.outgoing}` : `${t.incoming}`;
      const statusClass = rec.status === 'completed' ? 'status-completed' : (rec.status === 'paused' ? 'status-paused' : 'status-failed');

      const actionBtn = rec.status === 'completed' ?
        `<button class="control-btn" onclick="window.LANShareOpenFolder('')">${t.open_folder}</button>` : '-';

      tr.innerHTML = `
        <td><strong>${escapeHTML(rec.file_name)}</strong></td>
        <td>${dirIcon}</td>
        <td>${escapeHTML(rec.peer_name)} (${getOSIcon(rec.peer_os)})</td>
        <td>${formatBytes(rec.file_size)}</td>
        <td>${rec.average_speed ? rec.average_speed.toFixed(1) + ' MB/s' : '-'}</td>
        <td><span class="status-badge ${statusClass}">${rec.status}</span></td>
        <td>${dt}</td>
        <td>${actionBtn}</td>
      `;
      historyTbody.appendChild(tr);
    });
  }

  window.LANShareOpenFolder = async function (path) {
    try {
      await fetch('/api/open_folder', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: path })
      });
    } catch (e) {
      console.error('Failed to open folder', e);
    }
  };

  // Helper Functions
  function getOSIcon(os) {
    if (!os) return '💻';
    os = os.toLowerCase();
    if (os.includes('win')) return '🪟';
    if (os.includes('darwin') || os.includes('mac')) return '🍎';
    if (os.includes('linux')) return '🐧';
    return '💻';
  }

  function getOSLabel(os) {
    if (!os) return 'Unknown OS';
    os = os.toLowerCase();
    if (os.includes('win')) return 'Windows';
    if (os.includes('darwin') || os.includes('mac')) return 'macOS';
    if (os.includes('linux')) return 'Linux';
    return os;
  }

  function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function escapeHTML(str) {
    if (!str) return '';
    return str.replace(/[&<>'"]/g, 
      tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
    );
  }

})();
