import React, { useState } from "react";
import {
  Smile,
  Cat,
  Sparkles,
  Play,
  KeyRound,
  PackageOpen,
} from "lucide-react";
import { Action, Choices, Figure, HeroCopy, Mark } from "./ui.jsx";
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
  const [scene, setScene] = useState("cat");
  return (
    <section className="pi-hero meme-hero">
      <div className="pi-container pi-split">
        <HeroCopy
          title={
            <>
              有些话，
              <br />
              一张图就懂了。
            </>
          }
          description="说出你想怎么回，让 Agent 找到合适的表情，把原图带回聊天。"
        >
          <div className="pi-actions">
            <Action href="#install" icon={PackageOpen}>
              安装 Emomo
            </Action>
            <Action href="/account" tone="ghost" icon={KeyRound}>
              领取密钥
            </Action>
          </div>
        </HeroCopy>
        <article id="examples" className="meme-desk">
          <div className="meme-desk-title">
            <Smile size={25} />
            <h2>这句话，想怎么回？</h2>
          </div>
          <Choices
            label="选择表情或演示"
            tabs={scenes}
            value={scene}
            onChange={setScene}
          >
            {(s) =>
              s.id === "film" ? (
                <div className="meme-film">
                  <video
                    controls
                    playsInline
                    preload="metadata"
                    poster="/demo/emomo-v10-cover.webp"
                    aria-label="Emomo 使用演示"
                  >
                    <source src="/demo/emomo-v10.mp4" type="video/mp4" />
                  </video>
                  <a href="/demo/emomo-v10.mp4">打开完整演示</a>
                </div>
              ) : (
                <div className="meme-result">
                  <p>{s.ask}</p>
                  <Figure
                    src={s.src}
                    title={s.label + "表情"}
                    alt={s.alt}
                    width={1080}
                    height={s.id === "cat" ? 1087 : 1080}
                    priority
                  />
                  <small>真实图库示例</small>
                </div>
              )
            }
          </Choices>
        </article>
      </div>
    </section>
  );
}
