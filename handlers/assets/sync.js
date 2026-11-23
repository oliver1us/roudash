let activeContext = '';
let switching = false;
let ingressRequest = null;
let lastSuccess = '';
let routeRenewal = null;

// A rejected obsolete route never reached an API handler. Retry only this
// specific response, never a network error or a Kubernetes authorization error.
async function apiFetch(url, options = {}) {
    const mapping = ROUTES_MAP;
    const pathname = url.split('?')[0];
    const entry = Object.entries(mapping).find(([,path]) => pathname === path || pathname.startsWith(path + '/'));
    let res = await fetch(url, options);
    if (entry && res.status === 401 && res.headers.get('X-Routes-Expired') === '1') {
        if (!routeRenewal) {
            routeRenewal = fetch('/', {method:'HEAD', cache:'no-store'}).then(r => {
                const header = r.headers.get('X-Routes-Map');
                if (!r.ok || !header) throw new Error('Could not renew API routes');
                ROUTES_MAP = JSON.parse(header);
            }).finally(() => {routeRenewal = null;});
        }
        await routeRenewal;
        res = await fetch(ROUTES_MAP[entry[0]] + url.slice(entry[1].length), options);
    }
    const header = res.headers.get('X-Routes-Map');
    if (header && mapping === ROUTES_MAP) ROUTES_MAP = JSON.parse(header);
    return res;
}

function setRefreshMessage(message, failed = false) {
    const el = document.getElementById('lastRefresh');
    el.textContent = message;
    el.classList.toggle('failed', failed);
}

function resetContext(context) {
    ingressRequest?.controller.abort();
    ingressRequest = null;
    activeContext = context;
    lastSuccess = '';
    allIngresses = [];
    closeRightPanel();
    topology.clear();
    document.getElementById('loading').style.display = 'none';
    document.getElementById('refreshBtn').classList.remove('spinning');
    document.getElementById('refreshBtn').disabled = !context;
    document.getElementById('syncError').hidden = true;
    document.getElementById('searchInput').value = '';
    document.getElementById('namespaceSelect').value = 'all';
    setRefreshMessage('');
    initDashboard();
}

function fetchIngresses() {
    if (!activeContext || switching) return;
    if (ingressRequest) return ingressRequest.promise;
    const req = {controller:new AbortController()};
    ingressRequest = req;
    const btn = document.getElementById('refreshBtn');
    btn.disabled = true;
    btn.classList.add('spinning');
    setRefreshMessage('Updating…');
    if (!lastSuccess) document.getElementById('loading').style.display = 'flex';
    req.promise = (async () => {
        try {
            const res = await apiFetch(API.ingresses, {signal:req.controller.signal});
            const result = await res.json();
            if (ingressRequest !== req) return;
            if (!res.ok || result.status !== 'success' || !Array.isArray(result.data))
                throw new Error(result.message || `Unexpected response (${res.status})`);
            allIngresses = result.data;
            initDashboard();
            if (currentPanelState) {
                const {type, arg} = currentPanelState;
                const fresh = type === 'ingress'
                    ? allIngresses.find(i => i.namespace === arg.namespace && i.name === arg.name)
                    : arg;
                if (type === 'ingress' && !fresh) closeRightPanel();
                else renderDetailView(type, fresh, true, currentPanelState);
            }
            lastSuccess = new Date().toLocaleTimeString();
            setRefreshMessage('Updated ' + lastSuccess);
            document.getElementById('syncError').hidden = true;
        } catch (err) {
            if (ingressRequest !== req) return;
            const el = document.getElementById('syncError');
            el.textContent = 'Refresh failed: ' + err.message + (lastSuccess ? '. Showing data from ' + lastSuccess + '.' : '. Use Refresh to retry.');
            el.hidden = false;
            setRefreshMessage(lastSuccess ? 'Last update ' + lastSuccess : 'Update failed', true);
        } finally {
            if (ingressRequest === req) {
                ingressRequest = null;
                btn.disabled = false;
                btn.classList.remove('spinning');
                document.getElementById('loading').style.display = 'none';
            }
        }
    })();
    return req.promise;
}

async function activateConnection(url, context) {
    if (switching) return;
    switching = true;
    const select = document.getElementById('contextSwitcher');
    select.disabled = true;
    resetContext('');
    updateConnBadge('connecting', context, '');
    try {
        const res = await apiFetch(url, context ? {
            method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({context})
        } : {method:'POST'});
        const data = await res.json();
        if (!res.ok || data.status !== 'success') throw new Error(data.message || 'Connection failed');
    } finally {
        try {
            switching = false;
            await checkStatus();
        } finally {
            select.disabled = false;
        }
    }
}

function toggleAutoRefresh() {
    clearInterval(autoRefreshTimer);
    const on = document.getElementById('autoRefreshCheck').checked;
    document.getElementById('autoRefreshToggle').classList.toggle('active', on);
    autoRefreshTimer = on ? setInterval(() => {
        if (!document.hidden) fetchIngresses();
    }, 30000) : null;
}
document.addEventListener('visibilitychange', () => {
    if (!document.hidden && autoRefreshTimer) fetchIngresses();
});
