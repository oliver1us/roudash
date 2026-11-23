// The graph is derived from the same filtered Ingress list as the dashboard.
// Resource identity, rather than position in an API response, drives updates.
const topology = (() => {
    const palette = {
        cluster: '#f2c184', namespace: '#ad9ae2', ingress: '#8bbde1',
        service: '#96c7ab', host: '#e9ae89', plugin: '#da97b0'
    };
    const labels = {cluster:'Cluster', namespace:'Namespace', ingress:'Ingress', service:'Service', host:'Host', plugin:'Kong plugin'};
    let network, dots, lines, observer, selected, fitPending = true, detailed, showLabels = true;
    let model = new Map(), connections = new Map();
    const $ = id => document.getElementById(id);

    function init() {
        dots = new vis.DataSet();
        lines = new vis.DataSet();
        network = new vis.Network($('graphCanvas'), {nodes:dots, edges:lines}, {
            autoResize: false, layout: {improvedLayout: false, randomSeed: 17},
            nodes: {shape:'dot', borderWidth:0, borderWidthSelected:2, font:{face:'monospace', size:12, color:'#e6dfd3', strokeWidth:3, strokeColor:'#211e1a'}},
            edges: {smooth:false, width:1, color:{inherit:false, color:'#686055', highlight:'#ffd397'}},
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
            const color = lit ? palette[n.type] : '#514b42';
            const prev = dots.get(id);
            if (prev.label !== label || prev.color.background !== color)
                changes.push({id, label, color:{background:color, border:color, highlight:{background:palette[n.type],border:'#fff3df'}}});
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
            const color = palette[n.type];
            newDots.push({id, label:showLabels ? n.label : '', color:{background:color,border:color,highlight:{background:color,border:'#fff3df'}},
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
        $('graphLegend').replaceChildren(...Object.entries(palette).map(([type,color]) => {
            const item = document.createElement('span');
            item.className = 'legend-item';
            const dot = document.createElement('i');
            dot.className = 'legend-dot'; dot.style.background = color;
            item.append(dot, labels[type]); return item;
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
