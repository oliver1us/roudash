(() => {
    const sidebar = document.getElementById('leftSidebar');
    const handle = sidebar.querySelector('.sidebar-resizer');
    const mobile = matchMedia('(max-width: 800px)');
    const key = 'roudash.sidebarWidth';
    const min = 200;
    const max = () => Math.min(480, Math.floor(window.innerWidth * 0.4));
    const clamp = width => Math.max(min, Math.min(max(), Math.round(width)));
    let preferred = sidebar.offsetWidth;
    try {
        const saved = Number(localStorage.getItem(key));
        if (Number.isFinite(saved) && saved >= min) preferred = saved;
    } catch { /* Storage is optional. */ }

    function render() {
        if (mobile.matches) {
            sidebar.style.removeProperty('--sidebar-width');
            return;
        }
        const width = clamp(preferred);
        sidebar.style.setProperty('--sidebar-width', `${width}px`);
        handle.setAttribute('aria-valuemin', min);
        handle.setAttribute('aria-valuemax', max());
        handle.setAttribute('aria-valuenow', width);
        handle.setAttribute('aria-valuetext', `${width} pixels`);
    }
    function save() {
        try { localStorage.setItem(key, preferred); } catch { /* Keep the session choice. */ }
    }

    let offset = 0;
    handle.addEventListener('pointerdown', event => {
        if (!event.isPrimary || event.button !== 0) return;
        offset = event.clientX - sidebar.offsetWidth;
        handle.setPointerCapture(event.pointerId);
        event.preventDefault();
    });
    handle.addEventListener('pointermove', event => {
        if (mobile.matches || !handle.hasPointerCapture(event.pointerId)) return;
        preferred = clamp(event.clientX - offset);
        render();
    });
    handle.addEventListener('lostpointercapture', save);
    handle.addEventListener('keydown', event => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
        event.preventDefault();
        preferred = clamp(sidebar.offsetWidth + (event.key === 'ArrowRight' ? 16 : -16));
        render();
        save();
    });
    window.addEventListener('resize', render);
    render();
})();
