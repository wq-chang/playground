import { NavLink, NavLinkProps } from '@mantine/core';
import { createLink, LinkComponent } from '@tanstack/react-router';
import { forwardRef } from 'react';

type MantineNavLinkProps = {} & Omit<NavLinkProps, 'href'>;

const MantineNavLinkComponent = forwardRef<
  HTMLAnchorElement,
  MantineNavLinkProps
>((props, ref) => {
  return <NavLink ref={ref} {...props} />;
});
MantineNavLinkComponent.displayName = 'CustomNavLink';

const CreatedLinkComponent = createLink(MantineNavLinkComponent);

const CustomNavLink: LinkComponent<typeof MantineNavLinkComponent> = (
  props,
) => {
  return <CreatedLinkComponent preload="intent" {...props} />;
};

export default CustomNavLink;
