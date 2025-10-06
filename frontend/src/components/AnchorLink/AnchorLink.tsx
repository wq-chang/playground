import { Anchor, AnchorProps } from '@mantine/core';
import { createLink, LinkComponent } from '@tanstack/react-router';
import { forwardRef } from 'react';

type MantineAnchorProps = {} & Omit<AnchorProps, 'href'>;

const MantineLinkComponent = forwardRef<HTMLAnchorElement, MantineAnchorProps>(
  (props, ref) => {
    return <Anchor ref={ref} {...props} />;
  },
);
MantineLinkComponent.displayName = 'AnchorLink';

const CreatedLinkComponent = createLink(MantineLinkComponent);

const AnchorLink: LinkComponent<typeof MantineLinkComponent> = (props) => {
  return <CreatedLinkComponent preload="intent" {...props} />;
};

export default AnchorLink;
