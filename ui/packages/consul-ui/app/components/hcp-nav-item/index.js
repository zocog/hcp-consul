import Component from '@glimmer/component';
import { inject as service } from '@ember/service';

export default class HcpLinkItemComponent extends Component {
  @service('hcp-link-status') hcpLinkStatus;
  @service env;

  get hcpLink() {
    // TODO: How do I figure this out? From the linking API?
    return 'https://corn.com';
  }

  get accessedFromHcp() {
    // If the user has accessed consul from HCP managed consul, we do NOT want to display the
    // "HCP Consul Central↗️" link in the nav bar. As we're already displaying a BackLink from Hcp::Home
    return !!this.env.var('CONSUL_HCP_URL');
  }

  get shouldDisplayNewBadge() {
    return true;
  }
}
