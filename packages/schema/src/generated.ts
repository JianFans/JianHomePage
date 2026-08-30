/* 此文件由 pnpm schema:generate 生成，请勿手工修改。 */

export type HttpsUrl = string;
export type ContentTarget =
  | {
      kind: "internal";
      contentId: string;
    }
  | {
      kind: "external";
      link: PlatformLink;
    };
export type Asset = {
  id: string;
  kind: "image" | "gif" | "video" | "audio";
  src: MediaUrl;
  mimeType: "image/webp" | "image/gif" | "video/mp4" | "audio/wav" | "audio/mpeg";
  byteSize: number;
  width?: number;
  height?: number;
  durationSeconds?: number;
  posterAssetId?: string;
  alt: LocalizedText;
  rights: {
    source: LocalizedText;
    credit?: string;
    license?: string;
  };
  checksum: string;
} & Asset1;
export type MediaUrl = HttpsUrl | `/media/${string}`;
export type Asset1 =
  | {
      kind: "image";
      mimeType: "image/webp";
      [k: string]: unknown;
    }
  | {
      kind: "gif";
      mimeType: "image/gif";
      [k: string]: unknown;
    }
  | {
      kind: "video";
      mimeType: "video/mp4";
      [k: string]: unknown;
    }
  | {
      kind: "audio";
      mimeType: "audio/wav" | "audio/mpeg";
      [k: string]: unknown;
    };

export interface YujianContentSnapshot {
  schemaVersion: "1.0.0";
  releaseId: string;
  generatedAt: string;
  site: SiteConfig;
  homepage: Homepage;
  heroSlides: HeroSlide[];
  releases: Release[];
  tracks: Track[];
  videos: Video[];
  events: Event[];
  moments: Moment[];
  artist: ArtistProfile;
  assets: Asset[];
}
export interface SiteConfig {
  brand: LocalizedText;
  artistName: LocalizedText;
  defaultLocale: "zh-CN";
  /**
   * @minItems 2
   */
  supportedLocales: ["zh-CN" | "en", "zh-CN" | "en", ...("zh-CN" | "en")[]];
  canonicalUrl: HttpsUrl;
  socialLinks: PlatformLink[];
  seo: {
    title: LocalizedText;
    description: LocalizedText;
    ogAssetId: string;
  };
  futureNavigation: {
    id: string;
    label: LocalizedText;
    url: HttpsUrl;
    enabled: boolean;
  }[];
}
export interface LocalizedText {
  "zh-CN": string;
  en?: string;
}
export interface PlatformLink {
  provider: "qq-music" | "netease-music" | "weibo" | "bilibili" | "douyin" | "website" | "other";
  url: HttpsUrl;
  label?: LocalizedText;
}
export interface Homepage {
  /**
   * @minItems 1
   */
  sections: [
    HeroSection | MusicSection | VideoSection | EventSection | MomentSection | ArtistSection,
    ...(HeroSection | MusicSection | VideoSection | EventSection | MomentSection | ArtistSection)[]
  ];
}
export interface HeroSection {
  id: string;
  type: "hero";
  enabled: boolean;
  layoutVariant: "immersive";
  /**
   * @minItems 1
   * @maxItems 5
   */
  itemIds:
    | [string]
    | [string, string]
    | [string, string, string]
    | [string, string, string, string]
    | [string, string, string, string, string];
}
export interface MusicSection {
  id: string;
  type: "music";
  enabled: boolean;
  layoutVariant: "cover-reel";
  itemIds: string[];
  limit: number;
  moreLink?: PlatformLink;
}
export interface VideoSection {
  id: string;
  type: "video";
  enabled: boolean;
  layoutVariant: "asymmetric";
  itemIds: string[];
  limit: number;
}
export interface EventSection {
  id: string;
  type: "event";
  enabled: boolean;
  layoutVariant: "timeline";
  itemIds: string[];
  limit: number;
}
export interface MomentSection {
  id: string;
  type: "moment";
  enabled: boolean;
  layoutVariant: "mosaic";
  itemIds: string[];
  limit: number;
}
export interface ArtistSection {
  id: string;
  type: "artist";
  enabled: boolean;
  layoutVariant: "split";
  /**
   * @maxItems 1
   */
  itemIds: [] | ["artist_primary"];
}
export interface HeroSlide {
  id: string;
  mediaKind: "image" | "gif" | "video";
  assetId: string;
  mobileAssetId?: string;
  posterAssetId?: string;
  focalPoint: {
    x: number;
    y: number;
  };
  headline: LocalizedText;
  releaseId?: string;
  target?: ContentTarget;
  autoplay: boolean;
  startsAt?: string;
  endsAt?: string;
}
export interface Release {
  id: string;
  kind: "single" | "ep" | "album";
  title: LocalizedText;
  coverAssetId: string;
  releaseDate: string;
  /**
   * @minItems 1
   */
  trackIds: [string, ...string[]];
  platformLinks: PlatformLink[];
  featured: boolean;
}
export interface Track {
  id: string;
  releaseId: string;
  title: LocalizedText;
  durationSeconds: number;
  previewAssetId?: string;
  previewDurationSeconds?: number;
  platformLinks: PlatformLink[];
  credits: {
    role: LocalizedText;
    name: string;
  }[];
}
export interface Video {
  id: string;
  title: LocalizedText;
  posterAssetId: string;
  videoAssetId?: string;
  durationSeconds: number;
  /**
   * @minItems 1
   */
  platformLinks: [PlatformLink, ...PlatformLink[]];
  featured: boolean;
}
export interface Event {
  id: string;
  title: LocalizedText;
  dateTime: string;
  city: LocalizedText;
  venue: LocalizedText;
  status: "scheduled" | "sold-out" | "cancelled";
  detailUrl: HttpsUrl;
  posterAssetId?: string;
}
export interface Moment {
  id: string;
  assetId: string;
  caption?: LocalizedText;
  target?: ContentTarget;
}
export interface ArtistProfile {
  id: "artist_primary";
  name: LocalizedText;
  shortBio: LocalizedText;
  portraitAssetId: string;
  platformLinks: PlatformLink[];
}
