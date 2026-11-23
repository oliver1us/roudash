const selectedFilters = {Namespace: new Set(), Plugin: new Set(), Host: new Set()};

function filteredIngresses() {
    const {Namespace, Plugin, Host} = selectedFilters;
    const search = document.getElementById('searchInput').value.trim().toLowerCase();
    const matches = text => text.toLowerCase().includes(search);
    return allIngresses.filter(ing =>
        (!Namespace.size || Namespace.has(ing.namespace)) &&
        (!Plugin.size || ing.plugins.some(p => Plugin.has(p))) &&
        (!Host.size || ing.paths.some(p => Host.has(p.host))) &&
        (!search || matches(ing.name) || matches(ing.namespace) || matches(ing.ingressClass) ||
            ing.plugins.some(matches) ||
            ing.paths.some(p => matches(p.host) || matches(p.path) || matches(p.serviceName)))
    );
}

function populateSidebarFilters() {
    const namespaces = new Map(), plugins = new Map(), hosts = new Map();
    const count = (map, value) => map.set(value, (map.get(value) || 0) + 1);
    for (const ing of allIngresses) {
        count(namespaces, ing.namespace);
        ing.plugins.forEach(p => count(plugins, p));
        ing.paths.forEach(p => { if (p.host && p.host !== '*') count(hosts, p.host); });
    }
    for (const [kind, counts, inspect] of [
        ['Namespace', namespaces, showNamespaceDetail],
        ['Plugin', plugins, showPluginDetail],
        ['Host', hosts, showHostDetail]
    ]) {
        const list = document.getElementById(`folder${kind}s`);
        document.getElementById(`count${kind}s`).textContent = counts.size;
        list.replaceChildren();
        for (const value of [...counts.keys()].sort()) {
            const row = document.getElementById('filterOption').content.firstElementChild.cloneNode(true);
            const input = row.querySelector('input');
            input.dataset.kind = kind;
            input.value = value;
            input.checked = selectedFilters[kind].has(value);
            input.onchange = () => {
                if (input.checked) selectedFilters[kind].add(value);
                else selectedFilters[kind].delete(value);
                filterData();
            };
            row.querySelector('.option-name').textContent = value;
            row.querySelector('label').title = value;
            row.querySelector('.badge-count').textContent = counts.get(value);
            const button = row.querySelector('button');
            button.title = `Inspect ${kind.toLowerCase()}: ${value}`;
            button.setAttribute('aria-label', button.title);
            button.onclick = () => { closeMobileNav(); inspect(value); };
            list.appendChild(row);
        }
        if (!counts.size) {
            const empty = document.createElement('li');
            empty.textContent = `No ${kind.toLowerCase()}s found`;
            list.appendChild(empty);
        }
    }
}

function syncFilterCheckboxes() {
    for (const input of document.querySelectorAll('.folder-list input')) {
        input.checked = selectedFilters[input.dataset.kind].has(input.value);
    }
}

function resetWorkspaceFilters() {
    for (const values of Object.values(selectedFilters)) values.clear();
    document.getElementById('searchInput').value = '';
}

function clearWorkspaceFilters() {
    resetWorkspaceFilters();
    syncFilterCheckboxes();
    filterData();
    document.getElementById('searchInput').focus();
}

function renderFilterBar() {
    const chips = document.getElementById('filterChips');
    const searchInput = document.getElementById('searchInput');
    chips.replaceChildren();
    function addChip(kind, value, remove) {
        const chip = document.createElement('button');
        chip.type = 'button';
        chip.textContent = `${kind}: ${value} ×`;
        chip.setAttribute('aria-label', `Remove ${kind.toLowerCase()}: ${value}`);
        chip.onclick = () => {
            const index = Array.from(chips.children).indexOf(chip);
            remove();
            filterData();
            (chips.children[index] || chips.lastElementChild || searchInput).focus();
        };
        chips.appendChild(chip);
    }
    for (const [kind, values] of Object.entries(selectedFilters)) {
        for (const value of values) addChip(kind, value, () => {
            values.delete(value);
            syncFilterCheckboxes();
        });
    }
    if (searchInput.value.trim()) addChip('Search', searchInput.value.trim(), () => { searchInput.value = ''; });
    document.getElementById('clearSearch').style.display = searchInput.value ? 'block' : 'none';
    document.getElementById('noFilters').hidden = !!chips.childElementCount;
    document.getElementById('clearFilters').hidden = !chips.childElementCount;
}
