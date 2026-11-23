// The list permission already gives us each Ingress. Resolve other Kubernetes
// resources only when selected; hosts and counters refer to related Ingresses.
function renderManifest(state, ingresses) {
    const {type, arg} = state;
    const section = document.getElementById('manifestTemplate').content.firstElementChild.cloneNode(true);
    document.getElementById('panelContent').prepend(section);
    const select = section.querySelector('select');
    const note = section.querySelector('.manifest-note');
    const pre = section.querySelector('pre');
    const code = section.querySelector('code');
    const buttons = section.querySelectorAll('button');
    const choices = new Map();
    let object;

    function add(label, choice) {
        choices.set(label, choice);
        select.add(new Option(label, label));
    }
    if (type === 'namespace') add(`Namespace: ${arg}`, {kind:type, name:arg});
    if (type === 'service') {
        const [namespace, name] = arg.split('/');
        add(`Service: ${arg}`, {kind:type, namespace, name});
    }
    if (type === 'plugin') {
        for (const namespace of new Set(ingresses.map(i => i.namespace))) {
            add(`Plugin reference: ${namespace}/${arg}`, {kind:type, namespace, name:arg});
        }
    }
    for (const i of ingresses) {
        const prefix = type === 'ingress' ? 'Ingress' : 'Related Ingress';
        add(`${prefix}: ${i.namespace}/${i.name}`, {manifest:i.manifest});
    }

    function paint() {
        for (const button of buttons) {
            button.setAttribute('aria-pressed', String(button.textContent === state.format));
            button.disabled = !object;
        }
        pre.hidden = !object;
        code.textContent = !object ? '' : state.format === 'JSON'
            ? JSON.stringify(object, null, 2)
            : jsyaml.dump(object, {lineWidth:-1, noRefs:true});
    }

    let request;
    async function load() {
        request?.abort();
        state.resource = select.value;
        const choice = choices.get(state.resource);
        object = choice?.manifest;
        note.textContent = !choice ? 'No related manifests.' : '';
        paint();
        if (!choice || object) return;

        const controller = new AbortController();
        request = controller;
        note.textContent = 'Loading manifest…';
        try {
            const res = await apiFetch(`${getRoute('manifest')}?${new URLSearchParams(choice)}`, {signal:controller.signal});
            const data = await res.json();
            if (currentPanelState !== state || controller.signal.aborted) return;
            if (!res.ok) throw new Error(data.message || `HTTP ${res.status}`);
            object = data;
            note.textContent = '';
        } catch (err) {
            if (currentPanelState !== state || controller.signal.aborted) return;
            note.textContent = `Manifest unavailable: ${err.message}`;
            // A list-only kubeconfig can still inspect the referring Ingress.
            const fallback = [...choices].find(([, entry]) => entry.manifest);
            if (fallback) {
                select.value = fallback[0];
                object = fallback[1].manifest;
                note.textContent += ' Showing the related Ingress.';
            }
        }
        paint();
    }

    for (const button of buttons) button.onclick = () => {
        state.format = button.textContent;
        paint();
    };
    select.hidden = !choices.size;
    if (choices.has(state.resource)) select.value = state.resource;
    select.onchange = () => { state.format = 'YAML'; load(); };
    load();
}
