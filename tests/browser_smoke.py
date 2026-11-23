"""Browser regression test. Run against a fresh, local RouDash test instance."""
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit

from playwright.sync_api import expect, sync_playwright


class Kube(BaseHTTPRequestHandler):
    def do_GET(self):
        srv = self.server
        if self.path != "/apis/networking.k8s.io/v1/ingresses":
            self.send_error(404)
            return
        srv.requests += 1
        srv.started.set()
        srv.release.wait(10)
        name = srv.name
        route = {"path": "/", "pathType": "Prefix", "backend": {"service": {
            "name": "backend", "port": {"number": 8080}}}}
        item = {"metadata": {"name": name, "namespace": "apps",
                             "annotations": {"konghq.com/plugins": "cors"}},
                "spec": {"rules": [{"host": "web.example.test",
                                    "http": {"paths": [route, dict(route, path="/api")]}}]}}
        body = {"kind": "IngressList", "apiVersion": "networking.k8s.io/v1", "items": [item]}
        self.send_response(503 if srv.failed else 200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        try:
            self.wfile.write(json.dumps(body).encode())
        except (ConnectionResetError, BrokenPipeError):
            pass

    def log_message(self, *args):
        pass


def fixture(name):
    srv = ThreadingHTTPServer(("127.0.0.1", 0), Kube)
    srv.name, srv.requests, srv.failed = name, 0, False
    srv.started, srv.release = threading.Event(), threading.Event()
    srv.release.set()
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv


def run(url, servers):
    a, b = servers
    config = {"apiVersion": "v1", "kind": "Config", "current-context": a.name,
              "users": [{"name": "fixture", "user": {}}],
              "clusters": [{"name": s.name, "cluster": {"server": "http://127.0.0.1:%d" % s.server_port}} for s in servers],
              "contexts": [{"name": s.name, "context": {"cluster": s.name, "user": "fixture"}} for s in servers]}
    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        page = browser.new_page(viewport={"width": 1280, "height": 800})
        errors, external, docs = [], [], []
        page.on("pageerror", lambda error: errors.append(str(error)))
        def local_only(route):
            if urlsplit(route.request.url).netloc != urlsplit(url).netloc:
                external.append(route.request.url)
                route.abort()
            else:
                if route.request.is_navigation_request():
                    docs.append(route.request.url)
                route.continue_()
        page.route("**/*", local_only)
        page.goto(url)
        page.locator("#tabIncluster").click()
        expect(page.locator("#panelIncluster")).to_be_visible()
        page.locator("#tabUpload").click()
        page.locator("#fileInput").set_input_files({"name": "smoke.json", "mimeType": "application/json",
                                                  "buffer": json.dumps(config).encode()})
        page.locator("#uploadBtn").click()
        page.locator(".context-item").filter(has_text=a.name).click()
        page.locator("#activateBtn").click()
        expect(page.locator(".ingress-name")).to_have_text(a.name)
        expect(page.locator("#viewGraph")).to_be_hidden()
        # Observe the renderer through its public API, then interact with the
        # actual canvas. No test-only graph handles in the production app.
        page.evaluate("""() => {vis.Network = new Proxy(vis.Network, {
            construct(target,args) { return window.testNetwork = Reflect.construct(target,args) }
        })}""")
        page.locator("#btnGraph").click()
        expect(page.locator("#graphCanvas canvas")).to_be_visible()
        count = page.locator("#graphCount")
        expect(count).to_have_text("6 nodes · 5 links")
        page.locator("#graphDensity").click()
        expect(count).to_have_text("3 nodes · 2 links")
        page.locator("#graphDensity").click()
        page.locator("#sidebar-ns-apps").click()
        expect(page.locator("#viewGraph")).to_be_visible()
        page.locator("#searchInput").fill("missing")
        expect(page.locator("#graphEmpty")).to_be_visible()
        page.locator("#searchInput").fill("web.example")
        expect(count).to_have_text("6 nodes · 5 links")
        page.evaluate("""() => new Promise(resolve => {
            testNetwork.once('stabilized', () => resolve(true));
            testNetwork.stabilize(200);
        })""")
        page.locator("button", has_text="Fit view").click()
        point = page.evaluate("""testNetwork.canvasToDOM(testNetwork.getPosition('ingress:apps/west'))""")
        page.locator("#graphCanvas canvas").click(position=point)
        expect(page.locator("#panelTitle")).to_contain_text("Ingress Spec")
        page.locator(".close-btn").click()
        page.evaluate("""() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))""")
        page.evaluate("""testNetwork.moveTo({scale:1.47, position:{x:18,y:35}})""")
        scale = page.evaluate("testNetwork.getScale()")

        # Change only the browser's cached route to simulate a tab waking up
        # after multiple rotation intervals. Exercise the real 401 + HEAD path.
        page.evaluate("ROUTES_MAP.ingresses = '/expired-test-route'")
        with page.expect_request(lambda r: r.method == "HEAD" and r.url == url + "/"):
            page.locator("#refreshBtn").click()
        expect(page.locator("#lastRefresh")).to_contain_text("Updated")
        expect(page.locator("#searchInput")).to_have_value("web.example")
        actual = page.evaluate("testNetwork.getScale()")
        assert abs(actual - scale) < .001, (scale, actual)
        assert len(docs) == 1
        a.failed = True
        page.locator("#refreshBtn").click()
        expect(page.locator("#syncError")).to_contain_text("Showing data from")
        expect(count).to_have_text("6 nodes · 5 links")
        a.failed = False
        a.release.clear()
        a.started.clear()
        before = a.requests
        page.locator("#refreshBtn").click()
        assert a.started.wait(3)
        page.evaluate("void fetchIngresses()")
        assert a.requests == before + 1
        page.locator("#contextSwitcher").select_option("smoke:" + b.name)
        expect(page.locator("#breadcrumbCluster")).to_have_text(b.name)
        expect(page.locator("#syncError")).to_be_hidden()
        a.release.set()
        expect(page.locator("#lastRefresh")).to_contain_text("Updated")
        # Use the real refresh timer, advancing the browser clock only.
        page.clock.install()
        page.locator("#autoRefreshCheck").check()
        before = b.requests
        page_route = page.evaluate("API.ingresses")
        with page.expect_response(lambda r: r.request.method == "GET" and r.url.endswith(page_route)):
            page.clock.fast_forward(30000)
        assert b.requests == before + 1
        page.locator("#autoRefreshCheck").uncheck()
        page.set_viewport_size({"width": 390, "height": 844})
        page.locator(".sidebar-toggle").click()
        page.locator("#btnDashboard").click()
        expect(page.locator(".ingress-name")).to_have_text(b.name)
        page.locator(".ingress-card").click()
        expect(page.locator("#panelTitle")).to_contain_text("Ingress Spec")
        page.locator(".close-btn").click()
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
        assert not errors, errors
        assert not external, external
        browser.close()


if __name__ == "__main__":
    servers = [fixture("west"), fixture("east")]
    try:
        run(os.environ["ROUDASH_TEST_URL"].rstrip("/"), servers)
        print("PASS: graph, route renewal, error recovery, context race, autosync, mobile and offline assets")
    finally:
        for server in servers:
            server.release.set()
            server.shutdown()
            server.server_close()
