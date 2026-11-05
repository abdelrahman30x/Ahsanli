import React from "react";

interface CountryFlagProps {
  code: string;
  className?: string;
  width?: number;
  height?: number;
}

export default function CountryFlag({
  code,
  className = "",
  width = 24,
  height = 16,
}: CountryFlagProps) {
  const upper = (code || "").toUpperCase();

  switch (upper) {
    case "EG": // Egypt
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-eg ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="5.33" fill="#ce1126" />
          <rect y="5.33" width="24" height="5.33" fill="#ffffff" />
          <rect y="10.67" width="24" height="5.33" fill="#000000" />
          {/* Golden Eagle of Saladin */}
          <path
            d="M10.8 6.5 C10.8 6.1 11.4 5.7 12 5.7 C12.6 5.7 13.2 6.1 13.2 6.5 L13.5 7.8 L14.5 8.2 L14.2 9.5 L13.2 9.3 L13.2 10.2 L10.8 10.2 L10.8 9.3 L9.8 9.5 L9.5 8.2 L10.5 7.8 Z"
            fill="#c09a3e"
          />
          <circle cx="12" cy="7.2" r="0.8" fill="#a47d25" />
        </svg>
      );

    case "SA": // Saudi Arabia
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-sa ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="16" fill="#006c35" />
          {/* Shahada text approximation + sword */}
          <path
            d="M6 7.5 C8 6.8 16 6.8 18 7.5 M7 6.2 C9 5.8 15 5.8 17 6.2 M8 8.7 C10 8.3 14 8.3 16 8.7"
            stroke="#ffffff"
            strokeWidth="0.8"
            strokeLinecap="round"
            fill="none"
          />
          {/* Sword */}
          <path
            d="M6.5 10.5 L17.5 10.5 M6.5 10.5 L8 9.5 M6.5 10.5 L8 11.5"
            stroke="#ffffff"
            strokeWidth="0.7"
            strokeLinecap="round"
          />
        </svg>
      );

    case "JO": // Jordan
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-jo ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="5.33" fill="#000000" />
          <rect y="5.33" width="24" height="5.33" fill="#ffffff" />
          <rect y="10.67" width="24" height="5.33" fill="#007a3d" />
          {/* Red Chevron */}
          <polygon points="0,0 10,8 0,16" fill="#ce1126" />
          {/* 7-pointed star */}
          <polygon
            points="3.5,6.5 4,7.4 4.9,7.1 4.4,7.9 5.1,8.5 4.1,8.7 4.1,9.7 3.5,9 2.9,9.7 2.9,8.7 1.9,8.5 2.6,7.9 2.1,7.1 3,7.4"
            fill="#ffffff"
          />
        </svg>
      );

    case "AE": // United Arab Emirates
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-ae ${className}`}
          aria-hidden="true"
        >
          <rect x="6" width="18" height="5.33" fill="#00732f" />
          <rect x="6" y="5.33" width="18" height="5.33" fill="#ffffff" />
          <rect x="6" y="10.67" width="18" height="5.33" fill="#000000" />
          <rect width="6" height="16" fill="#ff0000" />
        </svg>
      );

    case "KW": // Kuwait
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-kw ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="5.33" fill="#007a3d" />
          <rect y="5.33" width="24" height="5.33" fill="#ffffff" />
          <rect y="10.67" width="24" height="5.33" fill="#ce1126" />
          <polygon points="0,0 6,4.5 6,11.5 0,16" fill="#000000" />
        </svg>
      );

    case "OM": // Oman
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-om ${className}`}
          aria-hidden="true"
        >
          <rect x="6" width="18" height="5.33" fill="#ffffff" />
          <rect x="6" y="5.33" width="18" height="5.33" fill="#db161b" />
          <rect x="6" y="10.67" width="18" height="5.33" fill="#008000" />
          <rect width="6" height="16" fill="#db161b" />
          {/* Khanjar emblem */}
          <path
            d="M2.2 2.5 C2.2 2 3.8 2 3.8 2.5 L3.8 4 C3.8 4.6 3 5 3 5 C3 5 2.2 4.6 2.2 4 Z"
            fill="#ffffff"
          />
        </svg>
      );

    case "BH": // Bahrain
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-bh ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="16" fill="#da291c" />
          <polygon
            points="0,0 6,0 8.5,1.6 6,3.2 8.5,4.8 6,6.4 8.5,8 6,9.6 8.5,11.2 6,12.8 8.5,14.4 6,16 0,16"
            fill="#ffffff"
          />
        </svg>
      );

    case "QA": // Qatar
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-qa ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="16" fill="#8d1b3d" />
          <polygon
            points="0,0 7,0 9.2,0.89 7,1.78 9.2,2.67 7,3.56 9.2,4.44 7,5.33 9.2,6.22 7,7.11 9.2,8 7,8.89 9.2,9.78 7,10.67 9.2,11.56 7,12.44 9.2,13.33 7,14.22 9.2,15.11 7,16 0,16"
            fill="#ffffff"
          />
        </svg>
      );

    case "IQ": // Iraq
      return (
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 16"
          width={width}
          height={height}
          className={`flag-svg flag-iq ${className}`}
          aria-hidden="true"
        >
          <rect width="24" height="5.33" fill="#ce1126" />
          <rect y="5.33" width="24" height="5.33" fill="#ffffff" />
          <rect y="10.67" width="24" height="5.33" fill="#000000" />
          {/* Takbir in green Kufic */}
          <path
            d="M8.5 7.8 L9.8 7.8 M10.5 7.8 L11.8 7.8 M12.5 7.8 L13.8 7.8 M14.5 7.8 L15.5 7.8"
            stroke="#007a3d"
            strokeWidth="1.2"
            strokeLinecap="round"
          />
        </svg>
      );

    default:
      return (
        <span
          className={`flag-fallback ${className}`}
          style={{
            display: "inline-grid",
            placeItems: "center",
            width,
            height,
            fontSize: "10px",
            fontWeight: 700,
            background: "#eee",
            borderRadius: "2px",
          }}
        >
          {upper.slice(0, 2)}
        </span>
      );
  }
}
