#!/usr/bin/perl

use strict;
use Test::More;
use Test::Differences;
use lib 't';
use Test_Netspoc;

my ($title, $in, $out1, $out2, $out3, $head1, $head2, $head3);

############################################################
my $title = 'Optimize subnet at secondary packet filter';
############################################################

$in = <<END;
network:sub = { ip = 10.1.7.32/27; subnet_of = network:customer; }

router:r = { interface:sub; interface:customer = { ip = 10.1.7.30; } }

network:customer = { ip = 10.1.7.0/24; }

router:gw = {
 managed = secondary;
 model = IOS_FW;
 interface:customer = { ip = 10.1.7.1;    hardware = outside;}
 interface:trans    = { ip = 10.1.3.1;   hardware = inside;}
}

network:trans = { ip = 10.1.3.0/24; }

router:b1 = {
 managed;
 model = Linux;
 interface:trans = {
  ip = 10.1.3.3;
  hardware = eth0;
 }
 interface:server = {
  ip = 10.1.2.1;
  hardware = eth1;
 }
}

network:server = {
 ip = 10.1.2.0/24;
 host:s10 = { ip = 10.1.2.10; }
 host:s11 = { ip = 10.1.2.11; }
}

protocol:Echo = icmp 8;

service:p1 = {
 user = network:sub;
 permit src = user; dst = host:s10; prt = protocol:Echo;
}

service:p2 = {
 user = network:customer;
 permit src = user; dst = host:s11; prt = protocol:Echo;
}
END

$out1 = <<END;
# [ Routing ]
ip route add 10.1.7.0/24 via 10.1.3.1
END

$out2 = <<END;
! [ Routing ]
ip route 10.1.7.32 255.255.255.224 10.1.7.30
ip route 10.1.2.0 255.255.255.0 10.1.3.3
END

$out3 = <<END;
! [ ACL ]
ip access-list extended outside_in
 permit ip 10.1.7.0 0.0.0.255 10.1.2.0 0.0.0.255
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];
$head2 = (split /\n/, $out2)[0];
$head3 = (split /\n/, $out3)[0];

eq_or_diff(get_block(compile($in), $head1, $head2, $head3), 
           $out1.$out2.$out3, $title);

############################################################
$title = 'Optimize subnet for protocol with flag dst_net';
############################################################

$in =~ s/managed = secondary/managed/ms;
$in =~ s/(protocol:Echo = icmp 8)/$1, dst_net/;

$out1 = <<END;
! [ ACL ]
ip access-list extended outside_in
 permit icmp 10.1.7.0 0.0.0.255 10.1.2.0 0.0.0.255 8
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
$title = 'Optimize subnet of NAT network in zone';
############################################################

$in = <<END;
network:customer = { 
 ip = 10.9.9.0/24; 
 nat:N = { ip = 10.1.7.0/24; dynamic; } 
 has_subnets; 
}

router:r = { 
 interface:customer; 
 interface:sub = { ip = 10.1.7.34; bind_nat = N; }
}

network:sub = { ip = 10.1.7.32/27; }

router:gw = {
 managed = secondary;
 model = IOS_FW;
 interface:sub   = { ip = 10.1.7.33; hardware = outside;}
 interface:trans = { ip = 10.1.3.1;  hardware = inside; }
}

network:trans = { ip = 10.1.3.0/24; }

router:b1 = {
 managed;
 model = Linux;
 interface:trans  = { ip = 10.1.3.3; hardware = eth0; }
 interface:server = { ip = 10.1.2.1; hardware = eth1; }
}

network:server = { ip = 10.1.2.0/24; }

protocol:Echo = icmp 8;

service:p1 = {
 user = network:sub;
 permit src = user; dst = network:server; prt = tcp 80;
}
END

$out1 = <<END;
! [ Routing ]
ip route 10.1.7.0 255.255.255.0 10.1.7.34
ip route 10.1.2.0 255.255.255.0 10.1.3.3
END

$out2 = <<END;
! [ ACL ]
ip access-list extended outside_in
 permit ip 10.1.7.0 0.0.0.255 10.1.2.0 0.0.0.255
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];
$head2 = (split /\n/, $out2)[0];

eq_or_diff(get_block(compile($in), $head1, $head2), $out1.$out2, $title);

############################################################

done_testing;
