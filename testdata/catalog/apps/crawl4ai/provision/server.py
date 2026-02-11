"""Minimal FastAPI server wrapping crawl4ai."""
import asyncio, json, os, uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.responses import HTMLResponse
from pydantic import BaseModel, Field
from typing import Optional
from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig, CacheMode

app = FastAPI(title="Crawl4AI", version="0.8.0")

class CrawlRequest(BaseModel):
    url: str
    word_count_threshold: int = Field(default=10)
    bypass_cache: bool = Field(default=False)
    css_selector: Optional[str] = None

class CrawlResponse(BaseModel):
    url: str
    success: bool
    markdown: Optional[str] = None
    cleaned_html: Optional[str] = None
    error: Optional[str] = None

@app.get("/health")
async def health():
    return {"status": "ok"}

@app.post("/crawl", response_model=CrawlResponse)
async def crawl(req: CrawlRequest):
    try:
        url = req.url.strip()
        if not url.startswith(("http://", "https://", "file://", "raw:")):
            url = "https://" + url
        req.url = url
        browser_cfg = BrowserConfig(headless=os.getenv("CRAWL4AI_HEADLESS", "true").lower() == "true")
        run_cfg = CrawlerRunConfig(
            word_count_threshold=req.word_count_threshold,
            cache_mode=CacheMode.BYPASS if req.bypass_cache else CacheMode.ENABLED,
            css_selector=req.css_selector,
        )
        async with AsyncWebCrawler(config=browser_cfg) as crawler:
            result = await crawler.arun(url=req.url, config=run_cfg)
            return CrawlResponse(
                url=req.url,
                success=result.success,
                markdown=result.markdown.raw_markdown if result.markdown else None,
                cleaned_html=result.cleaned_html,
                error=result.error_message if not result.success else None,
            )
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/playground", response_class=HTMLResponse)
async def playground():
    return """<!DOCTYPE html>
<html><head><title>Crawl4AI Playground</title>
<style>body{font-family:sans-serif;max-width:800px;margin:40px auto;padding:0 20px}
textarea,input{width:100%;padding:8px;margin:8px 0;box-sizing:border-box}
textarea{height:300px;font-family:monospace;font-size:13px}
button{background:#2563eb;color:#fff;border:none;padding:10px 24px;border-radius:6px;cursor:pointer;font-size:14px}
button:disabled{opacity:0.5}
pre{background:#f3f4f6;padding:16px;border-radius:8px;overflow:auto;max-height:500px;white-space:pre-wrap}
</style></head><body>
<h1>Crawl4AI Playground</h1>
<label>URL to crawl:</label>
<input id="url" type="text" placeholder="https://example.com" />
<button id="btn" onclick="doCrawl()">Crawl</button>
<h3>Result (Markdown):</h3>
<pre id="result">Enter a URL and click Crawl...</pre>
<script>
async function doCrawl(){
  const btn=document.getElementById('btn');
  btn.disabled=true; btn.textContent='Crawling...';
  let url=document.getElementById('url').value.trim();
  if(url && !url.match(/^(https?|file):\/\//)) { url='https://'+url; document.getElementById('url').value=url; }
  try{
    const r=await fetch('/crawl',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({url})});
    const d=await r.json();
    document.getElementById('result').textContent=d.success?d.markdown:(d.error||d.detail||'Unknown error');
  }catch(e){document.getElementById('result').textContent='Error: '+e.message}
  btn.disabled=false; btn.textContent='Crawl';
}
</script></body></html>"""

if __name__ == "__main__":
    uvicorn.run(app, host=os.getenv("CRAWL4AI_HOST", "0.0.0.0"),
                port=int(os.getenv("CRAWL4AI_API_PORT", "11235")))
