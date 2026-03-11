import type { SVGProps } from 'react';

interface OntologyForgeLogoProps extends SVGProps<SVGSVGElement> {
  size?: number;
}

/**
 * Ontology Forge Logo
 *
 * A hammer icon representing forging/shaping a semantic layer from raw data.
 * Line-style to match other icons in the UI.
 *
 * Uses currentColor for stroke, so it adapts to light/dark themes automatically.
 */
export default function OntologyForgeLogo({ size = 24, className, ...props }: OntologyForgeLogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      {...props}
    >
      {/* Hammer head */}
      <rect
        x="4"
        y="2"
        width="16"
        height="6"
        rx="1.5"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      {/* Handle — open-bottom U shape */}
      <path
        d="M10.5 8V20C10.5 21.1 11.17 22 12 22C12.83 22 13.5 21.1 13.5 20V8"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </svg>
  );
}
