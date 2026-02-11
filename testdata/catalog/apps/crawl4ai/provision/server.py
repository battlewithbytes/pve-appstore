"""Minimal FastAPI server wrapping crawl4ai."""
import os
import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.responses import HTMLResponse
from pydantic import BaseModel, Field
from typing import Optional
from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig, CacheMode

app = FastAPI(title="Crawl4AI", version="0.8.0")

PLAYGROUND_PATH = os.path.join(os.path.dirname(__file__), "playground.html")


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
    with open(PLAYGROUND_PATH) as f:
        return f.read()


if __name__ == "__main__":
    uvicorn.run(app, host=os.getenv("CRAWL4AI_HOST", "0.0.0.0"),
                port=int(os.getenv("CRAWL4AI_API_PORT", "11235")))
