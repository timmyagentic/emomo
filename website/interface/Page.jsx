import React, { useState } from "react";
import { Button } from "@heroui/react/button";
import { Cat, Sparkles, Play, KeyRound, PackageOpen } from "lucide-react";
import { Action, Choices, Figure } from "./ui.jsx";
export const site = {
  name: "Emomo",
  lang: "zh",
  nav: [
    ["表情示例", "#examples"],
    ["安装", "#install"],
    ["文档", "/docs"],
    ["我的账户", "/account"],
  ],
};
const scenes = [
  {
    id: "cat",
    label: "不想动",
    icon: Cat,
    ask: "来张嘴上收到、实际不想动的猫。",
    src: "/examples/c81be4b90f490a50015478c9afb8c86d0d30202ca0d20159f72d9fb35c9cbe6e.webp",
    alt: "低头的猫，配字：收到，卑职这就不办。",
  },
  {
    id: "dance",
    label: "开心",
    icon: Sparkles,
    ask: "要一张开心到原地起飞的图。",
    src: "/examples/caff9696392b35412c8c6849f32ffe3ab75e1ff3f8b2c241874f13bad0306df7.webp",
    alt: "开心跳舞的露比，配字：旋转、开心、跳跃。",
  },
  { id: "film", label: "看演示", icon: Play },
];
export function Hero() {
  const [view, select] = useState("all");
  const items = [
    { id: "all", label: "全部" },
    { id: "stills", label: "表情图" },
    { id: "film", label: "使用演示", icon: Play },
  ];
  const film = (
    <video
      controls
      playsInline
      preload="metadata"
      poster="/demo/emomo-v10-cover.webp"
      aria-label="Emomo 使用演示"
    >
      <source src="/demo/emomo-v10.mp4" type="video/mp4" />
    </video>
  );
  return (
    <section className="pi-hero meme-hero" aria-labelledby="hero-title">
      <div className="pi-container">
        <header className="meme-directory-head">
          <div>
            <h1 id="hero-title">Emomo 表情库</h1>
            <p>用一句话给 Agent 描述想回的表情，取回原图或动图。</p>
          </div>
          <div className="pi-actions">
            <Action href="#install" icon={PackageOpen}>
              安装 Emomo
            </Action>
            <Action href="/account" tone="ghost" icon={KeyRound}>
              领取密钥
            </Action>
          </div>
        </header>
        <div id="examples" className="meme-shelf">
          <Choices
            label="浏览表情与演示"
            tabs={items}
            value={view}
            onChange={select}
          >
            {(item) =>
              item.id === "film" ? (
                <div className="meme-projection">
                  {film}
                  <a href="/demo/emomo-v10.mp4">打开完整演示</a>
                </div>
              ) : (
                <div className="meme-grid">
                  {scenes
                    .filter((s) => s.src)
                    .map((s) => (
                      <figure className={"meme-print meme-" + s.id} key={s.id}>
                        <Figure
                          src={s.src}
                          title={s.label + "表情"}
                          alt={s.alt}
                          width={1080}
                          height={s.id === "cat" ? 1087 : 1080}
                          priority
                        />
                        <figcaption>
                          <h2>
                            {s.id === "cat"
                              ? "收到，卑职这就不办"
                              : "旋转、开心、跳跃"}
                          </h2>
                          <p>{s.ask}</p>
                        </figcaption>
                      </figure>
                    ))}
                  {item.id === "all" && (
                    <figure className="meme-print meme-video">
                      <Button
                        className="meme-poster"
                        onPress={() => select("film")}
                        aria-label="播放 Emomo 使用演示"
                      >
                        <img
                          src="/demo/emomo-v10-cover.webp"
                          width="640"
                          height="360"
                          alt=""
                        />
                        <Play aria-hidden="true" size={35} />
                      </Button>
                      <figcaption>
                        <h2>从一句话到一张图</h2>
                        <p>观看一次完整的表情搜索。</p>
                      </figcaption>
                    </figure>
                  )}
                </div>
              )
            }
          </Choices>
        </div>
      </div>
    </section>
  );
}
