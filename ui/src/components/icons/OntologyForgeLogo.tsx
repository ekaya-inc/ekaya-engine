import type { SVGProps } from 'react';

interface OntologyForgeLogoProps extends SVGProps<SVGSVGElement> {
  size?: number;
}

/**
 * Ontology Forge Logo
 *
 * A diamond-shaped knowledge graph (four connected nodes with cross connections)
 * standing on a forge/anvil base, representing the forging of a semantic layer
 * from raw data.
 *
 * Uses currentColor for stroke, so it adapts to light/dark themes automatically.
 */
export default function OntologyForgeLogo({ size = 24, className, ...props }: OntologyForgeLogoProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 195 195"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      {...props}
    >
      {/* Diamond outline: four nodes connected */}
      <path
        d="M97.5 20L35 85L97.5 150L160 85Z"
        stroke="currentColor"
        strokeWidth="12"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {/* Internal cross connections */}
      <path
        d="M97.5 20L97.5 150"
        stroke="currentColor"
        strokeWidth="12"
        strokeLinecap="round"
      />
      <path
        d="M35 85L160 85"
        stroke="currentColor"
        strokeWidth="12"
        strokeLinecap="round"
      />
      {/* Pillar from diamond to forge base */}
      <path
        d="M97.5 150V175"
        stroke="currentColor"
        strokeWidth="12"
        strokeLinecap="round"
      />
      {/* Forge/anvil base */}
      <path
        d="M25 175H170"
        stroke="currentColor"
        strokeWidth="14"
        strokeLinecap="round"
      />
    </svg>
  );
}
