// The graph is derived from the same filtered Ingress list as the dashboard.
// Resource identity, rather than position in an API response, drives updates.
const topology = (() => {
    const labels = {cluster:'Cluster', namespace:'Namespace', ingress:'Ingress', service:'Service', host:'Host', plugin:'Kong plugin'};
    let palette, dimColor, selectionBorder;
    let network, dots, lines, observer, selected, fitPending = true, detailed, showLabels = true;
    let model = new Map(), connections = new Map();
    const $ = id => document.getElementById(id);

    function themeOptions() {
        const style = getComputedStyle(document.documentElement);
        const css = name => style.getPropertyValue(name).trim();
        palette = Object.fromEntries(Object.keys(labels).map(type => [type, css('--color-' + type)]));
        dimColor = css('--graph-dim');
        selectionBorder = css('--graph-selection');
        return {
            nodes: {shape:'dot', borderWidth:0, borderWidthSelected:2, font:{face:'monospace', size:12, color:css('--graph-label'), strokeWidth:3, strokeColor:css('--bg-main')}},
            edges: {smooth:false, width:1, color:{inherit:false, color:css('--graph-edge'), highlight:css('--graph-edge-active'), hover:css('--graph-edge-active')}}
        };
    }

    function nodeColor(type, lit = true) {
        const color = lit ? palette[type] : dimColor;
        const highlight = {background:palette[type], border:selectionBorder};
        return {background:color, border:color, highlight, hover:highlight};
    }

    document.addEventListener('themechange', () => {
        if (!network) return;
        network.setOptions(themeOptions());
        paint();
        if (currentView !== 'graph') network.stopSimulation();
    });

    function init() {
        dots = new vis.DataSet();
        lines = new vis.DataSet();
        network = new vis.Network($('graphCanvas'), {nodes:dots, edges:lines}, {
            autoResize: false, layout: {improvedLayout: false, randomSeed: 17},
            ...themeOptions(),
            physics: {solver:'barnesHut', barnesHut:{springLength:90, gravitationalConstant:-1450, damping:0.26}, stabilization:{iterations:160, fit:false}},
            interaction: {hover:true, hideEdgesOnDrag:false},
        });
        network.on('click', ({nodes}) => {
            selected = nodes[0];
            paint();
            const n = model.get(selected);
            if (n) renderDetailView(n.type, n.detail);
            else closeRightPanel();
        });
        network.on('stabilized', () => {
            if (fitPending && model.size && currentView === 'graph') fit();
        });
        observer = new ResizeObserver(() => {
            if (currentView === 'graph' && $('graphCanvas').clientWidth) {
                network.setSize('100%', '100%');
                network.redraw();
            }
        });
        observer.observe($('graphCanvas'));
    }

    function fit() {
        if (!network || !model.size) return;
        network.fit({animation:false});
        fitPending = false;
    }

    function paint() {
        if (!network) return;
        const neighborhood = selected ? new Set([selected, ...network.getConnectedNodes(selected)]) : null;
        const changes = [];
        for (const [id, n] of model) {
            const lit = !neighborhood || neighborhood.has(id);
            const label = lit && (showLabels || !!neighborhood) ? n.label : '';
            const color = lit ? palette[n.type] : dimColor;
            const prev = dots.get(id);
            if (prev.label !== label || prev.color.background !== color)
                changes.push({id, label, color:nodeColor(n.type, lit)});
        }
        if (changes.length) dots.update(changes);
    }

    function update(list) {
        const next = new Map(), edges = new Map();
        const add = (type, key, label, detail) => {
            const id = type + ':' + key;
            next.set(id, {type, label, detail});
            return id;
        };
        const link = (from, to) => {
            const id = JSON.stringify([from,to]);
            edges.set(id, {id, from, to});
        };
        if (detailed === undefined && list.length) detailed = list.length <= 100;
        if (list.length) {
            const root = add('cluster', 'root', $('breadcrumbCluster').textContent, null);
            for (const ing of list) {
                const ns = add('namespace', ing.namespace, ing.namespace, ing.namespace);
                link(root, ns);
                const key = `${ing.namespace}/${ing.name}`;
                const ingress = add('ingress', key, ing.name, ing);
                link(ns, ingress);
                if (!detailed) continue;
                for (const p of ing.paths) {
                    if (p.host && p.host !== '*') link(add('host', p.host, p.host, p.host), ingress);
                    if (p.serviceName && p.serviceName !== 'N/A') {
                        const svc = `${ing.namespace}/${p.serviceName}`;
                        link(ingress, add('service', svc, p.serviceName, svc));
                    }
                }
                for (const p of ing.plugins) link(ingress, add('plugin', p, p, p));
            }
        }
        $('graphEmpty').hidden = !!next.size;
        $('graphCount').textContent = `${next.size} nodes · ${edges.size} links`;
        $('graphDensity').textContent = detailed === false ? 'Show connections' : 'Overview only';
        if (!next.size && !network) return;
        if (!network) init();
        if (selected && !next.has(selected)) { selected = undefined; closeRightPanel(); }
        const removedEdges = [...connections.keys()].filter(id => !edges.has(id));
        const removedNodes = [...model.keys()].filter(id => !next.has(id));
        if (removedEdges.length) lines.remove(removedEdges);
        if (removedNodes.length) dots.remove(removedNodes);
        const newDots = [];
        for (const [id,n] of next) {
            if (model.has(id)) continue;
            newDots.push({id, label:showLabels ? n.label : '', color:nodeColor(n.type),
                size:({cluster:15,namespace:11,ingress:8})[n.type] || 5});
        }
        if (newDots.length) dots.add(newDots);
        const newEdges = [...edges.values()].filter(e => !connections.has(e.id));
        if (newEdges.length) lines.add(newEdges);
        model = next;
        connections = edges;
        paint();
        if (fitPending && next.size) network.startSimulation();
    }

    function clear() {
        observer?.disconnect();
        network?.destroy();
        network = selected = detailed = undefined;
        model.clear(); connections.clear(); fitPending = true;
    }
    function legend() {
        $('graphLegend').replaceChildren(...Object.entries(labels).map(([type,label]) => {
            const item = document.createElement('span');
            item.className = 'legend-item';
            const dot = document.createElement('i');
            dot.className = 'legend-dot ' + type;
            item.append(dot, label); return item;
        }));
    }
    return {update, fit, clear, legend,
        focus: id => {
            if (!model.has(id)) return;
            selected = id;
            network.selectNodes([id]);
            network.focus(id, {scale:network.getScale(), animation:false});
            paint();
        },
        pause: () => network?.stopSimulation(),
        resume: () => { if (network) {network.redraw(); if (fitPending) network.startSimulation();} },
        toggleLabels: () => { showLabels = !showLabels; $('graphLabels').setAttribute('aria-pressed',String(showLabels)); paint(); },
        toggleDetail: () => { detailed = !detailed; filterData(); },
        clearFocus: () => { if (selected) { selected = undefined; network.unselectAll(); paint(); } }
    };
})();
