(() => {
    const key = 'roudash.theme';
    const root = document.documentElement;
    // Restore before the styles load so a returning workspace paints in its chosen theme.
    try {
        if (localStorage.getItem(key) === 'light') root.dataset.theme = 'light';
    } catch { /* Storage may be unavailable in restricted browsing contexts. */ }

    document.addEventListener('DOMContentLoaded', () => {
        const buttons = document.querySelectorAll('.theme-toggle');
        function updateButtons() {
            const light = root.dataset.theme === 'light';
            for (const button of buttons) {
                button.title = light ? 'Switch to dark theme' : 'Switch to light theme';
                button.setAttribute('aria-label', button.title);
                button.querySelector('use').setAttribute('href', `/assets/icons.svg#${light ? 'moon' : 'sun'}`);
            }
        }
        function toggle() {
            root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
            try { localStorage.setItem(key, root.dataset.theme); } catch { /* Keep the session choice. */ }
            updateButtons();
            document.dispatchEvent(new Event('themechange'));
        }
        buttons.forEach(button => button.addEventListener('click', toggle));
        updateButtons();
    });
})();
